package sound

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/assetutils"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/clientutils"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/response"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/atomicarray"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/assetdelivery"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/assets"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/games"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/publish"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/shardedmap"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/taskqueue"
)

const assetTypeID int32 = 3

const soundUploadRetryTries = 5

var ErrUnauthorized = errors.New("could not access the asset - you may be signed out, or the asset is private")

var errStopped = errors.New("reupload stopped")

func Reupload(ctx *context.Context, r *request.Request) {
	client := ctx.Client
	logger := ctx.Logger
	pauseController := ctx.PauseController
	resp := ctx.Response

	idsToUpload := len(r.IDs)
	var idsProcessed atomic.Int32

	defaultPlaceIDs := r.DefaultPlaceIDs
	defaultPlaceIDsMap := make(map[int64]struct{}, len(defaultPlaceIDs))
	for _, placeID := range defaultPlaceIDs {
		defaultPlaceIDsMap[placeID] = struct{}{}
	}

	var groupID int64
	if r.IsGroup {
		groupID = r.CreatorID
	}
	currentPlaceID := r.PlaceID

	creatorPlaceMap := shardedmap.New[*atomicarray.AtomicArray[int64]]()
	creatorMutexMap := shardedmap.New[*sync.RWMutex]()

	nClients := ctx.Clients.Len()
	uploadQueue := taskqueue.New[int64](time.Minute, config.GetInt("sound_uploads_per_minute")*nClients)
	permissionQueue := taskqueue.New[*assets.PermissionResponse](time.Minute, config.GetInt("sound_permissions_per_minute")*nClients)
	rateLimitPause := time.Duration(config.GetInt("rate_limit_pause_seconds")) * time.Second
	quiesce := assetutils.NewQuiesce()
	permissionRequest := assetutils.NewPermissionBodyFromIds([]int64{r.UniverseID})

	logger.Println("Reuploading sounds...")

	reporter := assetutils.NewReporter(logger, idsToUpload, &idsProcessed, resp)
	resp.SetProgress(idsToUpload, &idsProcessed)
	ctx.StartProgress()
	defer ctx.StopProgress()

	grantPermissions := func(uc *roblox.Client, newID int64) (*assets.PermissionResponse, error) {
		permissionHandler, err := assets.NewUpdatePermissionsHandler(uc, newID, permissionRequest)
		if err != nil {
			return nil, err
		}

		res := <-permissionQueue.QueueTask(func() (*assets.PermissionResponse, error) {
			return retry.Do(
				retry.NewOptions(retry.Tries(3)),
				func(try int) (*assets.PermissionResponse, error) {
					pauseController.WaitIfPaused()
					if try > 1 {
						uploadQueue.Limiter.Wait()
					}

					permissionReponse, err := permissionHandler()
					if err == nil {
						return permissionReponse, nil
					}

					if err == assets.UpdatePermissionErrors.ErrNotAuthenticated {
						return nil, &retry.ExitRetry{Err: err}
					} else {
						switch err.(type) {
						case *net.OpError, *net.DNSError:
							permissionQueue.Limiter.Decrement()
						}
					}

					return nil, &retry.ContinueRetry{Err: err}
				},
			)
		})
		return res.Result, res.Error
	}

	uploadAsset := func(wg *sync.WaitGroup, assetInfo *develop.AssetInfo, location string) {
		defer wg.Done()

		if ctx.Cancelled() {
			return
		}

		uploadClient := ctx.Clients.Next()

		oldName := assetInfo.Name
		resp.SetCurrent(assetInfo.Name)

		assetData, err := clientutils.GetRequest(uploadClient, location)
		if err != nil {
			reporter.UploadError("Failed to get asset data", assetInfo, err)
			return
		}

		uploadHandler, err := publish.NewUploadAudioHandler(uploadClient, assetInfo.Name, assetData, groupID)
		if err != nil {
			reporter.UploadError("Failed to get upload handler", assetInfo, err)
			return
		}

		newID, err := retry.Do(
			retry.NewOptions(
				retry.Tries(soundUploadRetryTries),
				retry.Delay(900*time.Millisecond),
			),
			func(_ int) (int64, error) {
				for {
					if ctx.Cancelled() {
						return 0, &retry.ExitRetry{Err: errStopped}
					}

					pauseController.WaitIfPaused()
					quiesce.Wait()

					res := <-uploadQueue.QueueTask(func() (int64, error) {
						uploadResponse, err := uploadHandler()
						if err != nil {
							return 0, err
						}
						return uploadResponse.ID, nil
					})
					id, err := res.Result, res.Error
					if err == nil {
						return id, nil
					}

					if errors.Is(err, publish.ErrRateLimited) {
						quiesce.Extend(rateLimitPause)
						continue
					}

					switch {
					case err == publish.UploadAudioErrors.ErrNotAuthenticated:
						clientutils.GetNewCookie(ctx, r, uploadClient, "cookie expired")
					case err == publish.UploadAudioErrors.ErrQuotaExceeded:
						if ctx.Cancel() {
							resp.SetStopped("audio upload limit reached")
							logger.Error("Audio upload limit reached - stopping reupload. The sounds that already reuploaded will still be applied.")
						}
						return 0, &retry.ExitRetry{Err: errStopped}
					case err == publish.UploadAudioErrors.ErrAccountModerated:
						assetutils.StopForModeration(ctx)
						return 0, &retry.ExitRetry{Err: errStopped}
					case err == publish.UploadAudioErrors.ErrModerated:
						assetInfo.Name = fmt.Sprintf("(%s) [Censored]", assetInfo.Name)
					default:
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
			assetutils.NoteUploadFailure(ctx, uploadClient)
			return
		}
		ctx.Clients.ReportSuccess(uploadClient)

		reporter.Success(assetInfo, newID)
		resp.AddItem(response.ResponseItem{
			OldID: assetInfo.ID,
			NewID: newID,
		})
		if err := ctx.Checkpoint.Record(assetInfo.ID, newID); err != nil {
			logger.Error("Failed to save reupload checkpoint: ", err)
		}

		if _, err = grantPermissions(uploadClient, newID); err != nil {
			message := fmt.Sprintf(">> %s(%d) failed to grant permission: ", assetInfo.Name, newID) + err.Error()
			if pauseController.IsPaused {
				color.Error.Fprintln(logger.History, message)
			} else {
				logger.Error(message)
			}
		} else if config.GetBool("print_successful_reuploads") || config.GetBool("verbose") {
			message := fmt.Sprintf(">> %s(%d) granted permission", assetInfo.Name, newID)
			if pauseController.IsPaused {
				color.Info.Fprintln(logger.History, message)
			} else {
				logger.Info(message)
			}
		}
	}

	getCreatorPlaceCache := func(creatorID int64, creatorType string) (*atomicarray.AtomicArray[int64], error) {
		creatorShard, exists := creatorPlaceMap.GetShard(creatorType)
		mutexShard, _ := creatorMutexMap.GetShard(creatorType)
		if !exists {
			creatorShard = creatorPlaceMap.NewShard(creatorType)
			mutexShard = creatorMutexMap.NewShard(creatorType)
		}

		if cache, cacheExists := creatorShard.Get(creatorID); cacheExists {
			return cache, nil
		}

		mutex, mutexExists := mutexShard.Get(creatorID)
		if !mutexExists {
			mutex = &sync.RWMutex{}
			mutexShard.Set(creatorID, mutex)
		}

		mutex.Lock()
		defer mutex.Unlock()

		if cache, cacheExists := creatorShard.Get(creatorID); cacheExists {
			return cache, nil
		}

		var resp *games.GamesResponse
		var err error
		for {
			pauseController.WaitIfPaused()
			quiesce.Wait()

			if creatorType == "Group" {
				resp, err = games.GroupGames(client, creatorID)
			} else {
				resp, err = games.UserGames(client, creatorID)
			}

			if err != nil && errors.Is(err, games.ErrRateLimited) {
				quiesce.Extend(rateLimitPause)
				continue
			}
			break
		}
		if err != nil {
			return nil, err
		}

		ids := make([]int64, 0, len(defaultPlaceIDs)+len(resp.Data))
		ids = append(ids, defaultPlaceIDs...)
		for _, placeInfo := range resp.Data {
			rootPlaceID := placeInfo.RootPlace.ID

			if _, exists := defaultPlaceIDsMap[rootPlaceID]; exists {
				continue
			}

			ids = append(ids, rootPlaceID)
		}

		cache := atomicarray.New(&ids)
		creatorShard.Set(creatorID, cache)
		mutexShard.Remove(creatorID)
		return cache, nil
	}

	getAssetLocations := func(body []*assetdelivery.AssetRequestItem, placeID int64) ([]*assetdelivery.AssetLocation, error) {
		handler, err := assetdelivery.NewBatchHandler(client, body, placeID)
		if err != nil {
			return nil, err
		}

		return retry.Do(
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
							clientutils.GetNewCookie(ctx, r, client, "cookie expired")
							return locations, &retry.ContinueRetry{Err: ErrUnauthorized}
						}
					}

					return locations, nil
				}
			},
		)
	}

	batchUpload := func(wg *sync.WaitGroup, creatorID int64, creatorType string, creatorAssets []*develop.AssetInfo) {
		defer wg.Done()

		if ctx.Cancelled() {
			return
		}

		placeCache, err := getCreatorPlaceCache(creatorID, creatorType)
		if err != nil {
			reporter.BatchError(len(creatorAssets), "Failed to get creator places", err)
			return
		}

		assetInfoMap := make(map[int64]*develop.AssetInfo)
		ids := make([]int64, len(creatorAssets))
		for i, assetInfo := range creatorAssets {
			ids[i] = assetInfo.ID
			assetInfoMap[assetInfo.ID] = assetInfo
		}
		body := assetutils.NewBatchBodyFromIDs(ids)

		var uploadWG sync.WaitGroup
		lastLocationErrorByAssetID := make(map[int64]string)
		for _, placeID := range placeCache.Load() {
			assetLocations, err := getAssetLocations(body, placeID)
			if err != nil {
				reporter.BatchError(len(body), "Failed to get asset locations", err)
				return
			}

			for i, assetLocation := range assetLocations {
				if len(assetLocation.Locations) > 0 || i >= len(body) {
					continue
				}
				if errs := assetLocation.Errors; len(errs) > 0 {
					lastLocationErrorByAssetID[body[i].AssetID] = errs[0].Message
				}
			}

			var hadSuccess bool
			for assetIndex, assetLocation := range slices.Backward(assetLocations) {
				if len(assetLocation.Locations) == 0 {
					continue
				}
				hadSuccess = true

				assetID := body[assetIndex].AssetID
				body = slices.Delete(body, assetIndex, assetIndex+1)

				uploadWG.Add(1)
				go uploadAsset(&uploadWG, assetInfoMap[assetID], assetLocation.Locations[0].Location)
			}
			if hadSuccess {
				atomicarray.MoveToFront(placeCache, placeID)
			}
			if len(body) == 0 {
				break
			}
		}

		if !ctx.Cancelled() {
			for _, req := range body {
				assetInfo := assetInfoMap[req.AssetID]
				if lastErr, ok := lastLocationErrorByAssetID[req.AssetID]; ok && lastErr != "" {
					reporter.UploadError("Failed to get asset location", assetInfo, lastErr)
					continue
				}
				reporter.UploadError("Failed to get asset location", assetInfo, "no download URL from any place")
			}
		}

		uploadWG.Wait()
	}

	batchProcess := func(wg *sync.WaitGroup, filteredInfo []*develop.AssetInfo) {
		defer wg.Done()

		if ctx.Cancelled() {
			return
		}

		filteredInfoLength := len(filteredInfo)
		if filteredInfoLength == 0 {
			return
		}

		ids := make([]int64, filteredInfoLength)
		for i, assetInfo := range filteredInfo {
			ids[i] = assetInfo.ID
		}
		body := assetutils.NewBatchBodyFromIDs(ids)

		assetLocations, err := getAssetLocations(body, currentPlaceID)
		if err != nil {
			reporter.BatchError(filteredInfoLength, "Failed to get asset locations to see permissions", err)
		}

		unownedAssets := make([]*develop.AssetInfo, 0)
		for i, location := range assetLocations {
			if len(location.Locations) > 0 {
				continue
			}

			unownedAssets = append(unownedAssets, filteredInfo[i])
		}

		if owned := filteredInfoLength - len(unownedAssets); owned > 0 {
			idsProcessed.Add(int32(owned))
		}

		CreatorAssets := assetutils.GroupByCreator(unownedAssets)

		var uploadWG sync.WaitGroup
		for creatorType, creatorAssetMap := range CreatorAssets {
			uploadWG.Add(len(creatorAssetMap))

			for creatorID, creatorAssets := range creatorAssetMap {
				go batchUpload(&uploadWG, creatorID, creatorType, creatorAssets)
			}
		}
		uploadWG.Wait()
	}

	kept, report := assetutils.Classify(ctx, r, quiesce, rateLimitPause, "sounds", assetTypeID)
	idsProcessed.Add(int32(report.Total()))
	resp.SetSkipSummary(report.Lines("sounds"), report.AlreadyYours+report.WrongType+report.NotFound)
	for _, line := range report.Lines("sounds") {
		logger.Println(line)
	}
	if len(kept) == 0 {
		return
	}

	var wg sync.WaitGroup
	chunks := assetutils.ChunkAssets(kept, assetutils.AssetsInfoChunkSize)
	wg.Add(len(chunks))
	for _, chunk := range chunks {
		go batchProcess(&wg, chunk)
	}
	wg.Wait()
}
