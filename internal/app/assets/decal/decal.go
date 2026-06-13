package decal

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/assetutils"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/clientutils"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/response"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/assetdelivery"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/ide"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/taskqueue"
)

const decalAssetTypeID int32 = 13
const imageAssetTypeID int32 = 1
const decalUploadRetryTries = 5

var ErrUnauthorized = errors.New("could not access the asset - you may be signed out, or the asset is private")
var ErrNoImageID = errors.New("could not find the underlying image inside this decal")
var ErrImageUnavailable = errors.New("Roblox returned no download location for the decal's image")

var decalImageIDPattern = regexp.MustCompile(`(?:\?id=|rbxassetid://)(\d+)`)

func Reupload(ctx *context.Context, r *request.Request) {
	logger := ctx.Logger
	pauseController := ctx.PauseController
	resp := ctx.Response

	idsToUpload := len(r.IDs)
	var idsProcessed atomic.Int32

	var groupID int64
	if r.IsGroup {
		groupID = r.CreatorID
	}

	uploadQueue := taskqueue.New[int64](time.Minute, config.GetInt("decal_uploads_per_minute")*ctx.Clients.Len())
	rateLimitPause := time.Duration(config.GetInt("rate_limit_pause_seconds")) * time.Second
	quiesce := assetutils.NewQuiesce()

	logger.Println("Reuploading decals...")

	reporter := assetutils.NewReporter(logger, idsToUpload, &idsProcessed, resp)
	resp.SetProgress(idsToUpload, &idsProcessed)
	ctx.StartProgress()
	defer ctx.StopProgress()

	uploadAsset := func(wg *sync.WaitGroup, assetInfo *develop.AssetInfo, location string) {
		defer wg.Done()

		if ctx.Cancelled() {
			return
		}

		client := ctx.Clients.Next()

		oldName := assetInfo.Name
		resp.SetCurrent(assetInfo.Name)

		assetData, err := clientutils.GetRequest(client, location)
		if err != nil {
			reporter.UploadError("Failed to get asset data", assetInfo, err)
			return
		}

		imageData, err := resolveImageData(client, assetData)
		if err != nil {
			reporter.UploadError("Failed to read the decal's image", assetInfo, err)
			return
		}

		contentType := detectImageContentType(imageData.Bytes())
		uploadHandler, err := ide.NewUploadDecalHandler(client, assetInfo.Name, "", imageData, contentType, groupID)
		if err != nil {
			reporter.UploadError("Failed to get upload handler", assetInfo, err)
			return
		}

		newID, err := retry.Do(
			retry.NewOptions(
				retry.Tries(decalUploadRetryTries),
				retry.Delay(900*time.Millisecond),
			),
			func(_ int) (int64, error) {
				for {
					if ctx.Cancelled() {
						return 0, &retry.ExitRetry{Err: assetutils.ErrStopped}
					}

					pauseController.WaitIfPaused()
					quiesce.Wait()

					res := <-uploadQueue.QueueTask(uploadHandler)
					id, err := res.Result, res.Error
					if err == nil {
						return id, nil
					}

					if errors.Is(err, ide.ErrRateLimited) {
						quiesce.Extend(rateLimitPause)
						continue
					}

					if errors.Is(err, ide.ErrAccountModerated) {
						assetutils.StopForModeration(ctx)
						return 0, &retry.ExitRetry{Err: assetutils.ErrStopped}
					}

					if err == ide.UploadDecalErrors.ErrNotLoggedIn {
						clientutils.GetNewCookie(ctx, r, client, "cookie expired")
					} else if err == ide.UploadDecalErrors.ErrInappropriateName {
						assetInfo.Name = fmt.Sprintf("(%s) [Censored]", assetInfo.Name)
					} else {
						switch err.(type) {
						case *net.OpError, *net.DNSError:
							uploadQueue.Limiter.Decrement()
						}
					}

					reporter.Retry(assetInfo, err)
					return 0, &retry.ContinueRetry{Err: err}
				}
			},
		)
		if err != nil {
			assetInfo.Name = oldName
			if ctx.Cancelled() {
				return
			}
			reporter.UploadError("Failed to upload", assetInfo, err)
			assetutils.NoteUploadFailure(ctx, client)
			return
		}
		ctx.Clients.ReportSuccess(client)

		reporter.Success(assetInfo, newID)
		resp.AddItem(response.ResponseItem{
			OldID: assetInfo.ID,
			NewID: newID,
		})
		if err := ctx.Checkpoint.Record(assetInfo.ID, newID); err != nil {
			logger.Error("Failed to save reupload checkpoint: ", err)
		}
	}

	batchProcess := func(wg *sync.WaitGroup, filteredInfo []*develop.AssetInfo) {
		defer wg.Done()

		filteredInfoLength := len(filteredInfo)
		if filteredInfoLength == 0 {
			return
		}

		batchClient := ctx.Clients.Next()

		ids := make([]int64, filteredInfoLength)
		for i, assetInfo := range filteredInfo {
			ids[i] = assetInfo.ID
		}
		body := assetutils.NewBatchBodyFromIDs(ids)

		handler, err := assetdelivery.NewBatchHandler(batchClient, body)
		if err != nil {
			reporter.BatchError(filteredInfoLength, "Failed to get batch asset delivery handler", err)
			return
		}

		assetLocations, err := retry.Do(
			retry.NewOptions(retry.Tries(3)),
			func(_ int) ([]*assetdelivery.AssetLocation, error) {
				for {
					pauseController.WaitIfPaused()
					quiesce.Wait()

					locations, err := handler()
					if err != nil {
						if errors.Is(err, assetdelivery.ErrRateLimited) {
							quiesce.Extend(rateLimitPause)
							continue
						}
						logger.Verbose(fmt.Sprintf("Retrying asset-location lookup: %v", err))
						return locations, &retry.ContinueRetry{Err: err}
					}

					for _, assetLocation := range locations {
						errs := assetLocation.Errors
						if len(errs) == 0 {
							continue
						}
						if errs[0].Message == "Authentication required to access Asset." {
							clientutils.GetNewCookie(ctx, r, batchClient, "cookie expired")
							return locations, &retry.ContinueRetry{Err: ErrUnauthorized}
						}
					}

					return locations, nil
				}
			},
		)
		if err != nil {
			reporter.BatchError(filteredInfoLength, "Failed to get asset locations", err)
			return
		}

		var uploadWG sync.WaitGroup
		uploadWG.Add(filteredInfoLength)
		for i, assetInfo := range filteredInfo {
			locationInfo := assetLocations[i]

			if errs := locationInfo.Errors; len(errs) > 0 {
				reporter.UploadError("Failed to get asset location for", assetInfo, errs[0].Message)
				uploadWG.Done()
				continue
			}

			if len(locationInfo.Locations) == 0 {
				reporter.UploadError("Failed to get asset location for", assetInfo, "no asset location returned")
				uploadWG.Done()
				continue
			}

			location := locationInfo.Locations[0].Location
			go uploadAsset(&uploadWG, assetInfo, location)
		}
		uploadWG.Wait()
	}

	kept, report := assetutils.Classify(ctx, r, quiesce, rateLimitPause, "decals", decalAssetTypeID, imageAssetTypeID)
	idsProcessed.Add(int32(report.Total()))
	resp.SetSkipSummary(report.Lines("decals"), report.AlreadyYours+report.WrongType+report.NotFound)
	for _, line := range report.Lines("decals") {
		logger.Println(line)
	}
	if len(kept) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, chunk := range assetutils.ChunkAssets(kept, assetutils.AssetsInfoChunkSize) {
		wg.Add(1)
		go batchProcess(&wg, chunk)
	}
	wg.Wait()
}

func resolveImageData(client *roblox.Client, data *bytes.Buffer) (*bytes.Buffer, error) {
	if !looksLikeXML(data.Bytes()) {
		return data, nil
	}

	match := decalImageIDPattern.FindSubmatch(data.Bytes())
	if match == nil {
		return nil, ErrNoImageID
	}
	imageID, err := strconv.ParseInt(string(match[1]), 10, 64)
	if err != nil {
		return nil, ErrNoImageID
	}

	location, err := resolveImageLocation(client, imageID)
	if err != nil {
		return nil, err
	}

	return clientutils.GetRequest(client, location)
}

func resolveImageLocation(client *roblox.Client, imageID int64) (string, error) {
	handler, err := assetdelivery.NewBatchHandler(client, assetutils.NewBatchBodyFromIDs([]int64{imageID}))
	if err != nil {
		return "", err
	}

	return retry.Do(
		retry.NewOptions(retry.Tries(3)),
		func(_ int) (string, error) {
			locations, err := handler()
			if err != nil {
				return "", &retry.ContinueRetry{Err: err}
			}
			if len(locations) == 0 || len(locations[0].Locations) == 0 {
				return "", &retry.ExitRetry{Err: ErrImageUnavailable}
			}
			return locations[0].Locations[0].Location, nil
		},
	)
}

func looksLikeXML(b []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(b), []byte("<"))
}

func detectImageContentType(b []byte) string {
	contentType := http.DetectContentType(b)
	if i := strings.IndexByte(contentType, ';'); i != -1 {
		contentType = contentType[:i]
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "image/png"
	}
	return contentType
}
