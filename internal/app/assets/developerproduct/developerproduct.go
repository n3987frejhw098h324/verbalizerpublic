package developerproduct

import (
	"errors"
	"fmt"
	"net"
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
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/developerproducts"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/thumbnails"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/taskqueue"
)

const createRetryTries = 5

var errStopped = errors.New("reupload stopped")

func Reupload(ctx *context.Context, r *request.Request) {
	logger := ctx.Logger
	pauseController := ctx.PauseController
	resp := ctx.Response

	total := len(r.IDs)
	var processed atomic.Int32

	nClients := ctx.Clients.Len()
	rateLimitPause := time.Duration(config.GetInt("rate_limit_pause_seconds")) * time.Second
	quiesce := assetutils.NewQuiesce()
	detailsQueue := taskqueue.New[*developerproducts.Details](time.Minute, config.GetInt("item_details_per_minute")*nClients)
	createQueue := taskqueue.New[int64](time.Minute, config.GetInt("developerproduct_creates_per_minute")*nClients)

	logger.Println("Reuploading developer products...")

	reporter := assetutils.NewReporter(logger, total, &processed, resp)

	var lookedUp atomic.Int32
	resp.SetProgress(total, &lookedUp)
	resp.SetCurrent("looking up developer products")
	ctx.StartProgress()
	defer ctx.StopProgress()

	fetchDetails := func(id int64) (*developerproducts.Details, error) {
		backoff := assetutils.NewRateLimitBackoff(quiesce, rateLimitPause, logger, fmt.Sprintf("developer product %d", id))
		return retry.Do(
			retry.NewOptions(retry.Tries(3)),
			func(_ int) (*developerproducts.Details, error) {
				for {
					pauseController.WaitIfPaused()
					quiesce.Wait()

					res := <-detailsQueue.QueueTask(func() (*developerproducts.Details, error) {
						return developerproducts.GetDetails(ctx.Clients.Next(), id)
					})
					details, err := res.Result, res.Error
					if err == nil {
						return details, nil
					}
					if errors.Is(err, developerproducts.Errors.ErrRateLimited) {
						if !backoff.Wait() {
							return nil, &retry.ExitRetry{Err: err}
						}
						continue
					}
					if errors.Is(err, developerproducts.Errors.ErrNotFound) || errors.Is(err, developerproducts.Errors.ErrWrongType) {
						return nil, &retry.ExitRetry{Err: err}
					}
					logger.Verbose(fmt.Sprintf("Retrying details lookup for %d: %v", id, err))
					return nil, &retry.ContinueRetry{Err: err}
				}
			},
		)
	}

	kept, report := classify(ctx, r, fetchDetails, &lookedUp)

	resp.SetProgress(total, &processed)
	resp.SetCurrent("")
	processed.Add(int32(report.Total()))
	resp.SetSkipSummary(report.Lines("developer products"), report.AlreadyYours+report.WrongType+report.NotFound)
	for _, line := range report.Lines("developer products") {
		logger.Println(line)
	}
	if len(kept) == 0 {
		return
	}

	createOne := func(wg *sync.WaitGroup, details *developerproducts.Details) {
		defer wg.Done()

		if ctx.Cancelled() {
			return
		}

		client := ctx.Clients.Next()
		resp.SetCurrent(details.Name)
		info := &develop.AssetInfo{ID: details.ProductID, Name: details.Name}

		icon, err := clientutils.FetchIcon(client, func() (string, error) {
			return thumbnails.DeveloperProductIcon(client, details.TargetID)
		})
		if err != nil {
			reporter.UploadError("Failed to get developer product icon", info, err)
			return
		}
		const contentType = "image/png"

		name := details.Name
		backoff := assetutils.NewRateLimitBackoff(quiesce, rateLimitPause, logger, fmt.Sprintf("developer product %q", name))
		newID, err := retry.Do(
			retry.NewOptions(retry.Tries(createRetryTries), retry.Delay(900*time.Millisecond)),
			func(_ int) (int64, error) {
				for {
					if ctx.Cancelled() {
						return 0, &retry.ExitRetry{Err: errStopped}
					}
					pauseController.WaitIfPaused()
					quiesce.Wait()

					res := <-createQueue.QueueTask(func() (int64, error) {
						return developerproducts.Create(client, r.UniverseID, name, details.Description, icon, contentType)
					})
					id, err := res.Result, res.Error
					if err == nil {
						return id, nil
					}
					if errors.Is(err, developerproducts.Errors.ErrRateLimited) {
						if !backoff.Wait() {
							return 0, &retry.ExitRetry{Err: err}
						}
						continue
					}
					switch {
					case errors.Is(err, developerproducts.Errors.ErrNotAuthenticated):
						clientutils.GetNewCookie(ctx, r, client, "cookie expired")
					case errors.Is(err, developerproducts.Errors.ErrModerated):
						name = fmt.Sprintf("(%s) [Censored]", name)
					case errors.Is(err, developerproducts.Errors.ErrTokenInvalid):
						default:
						switch err.(type) {
						case *net.OpError, *net.DNSError:
							createQueue.Limiter.Decrement()
						}
					}
					reporter.Retry(info, err)
					return 0, &retry.ContinueRetry{Err: err}
				}
			},
		)
		if err != nil {
			if ctx.Cancelled() {
				return
			}
			reporter.UploadError("Failed to create developer product", info, err)
			assetutils.NoteUploadFailure(ctx, client)
			return
		}
		ctx.Clients.ReportSuccess(client)

		if details.Price > 0 {
			if err := setPrice(ctx, client, r.UniverseID, newID, name, details, quiesce, rateLimitPause); err != nil {
				if errors.Is(err, developerproducts.Errors.ErrOffsale) {
					logger.Verbose(fmt.Sprintf(">> %s(%d) created but left off-sale (price update returned 204)", name, newID))
				} else {
					logger.Error(fmt.Sprintf(">> %s(%d) created but price could not be set: %v", name, newID, err))
				}
			}
		}

		reporter.Success(info, newID)
		resp.AddItem(response.ResponseItem{OldID: details.ProductID, NewID: newID})
		if err := ctx.Checkpoint.Record(details.ProductID, newID); err != nil {
			logger.Error("Failed to save reupload checkpoint: ", err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(len(kept))
	for _, details := range kept {
		go createOne(&wg, details)
	}
	wg.Wait()
}

func setPrice(ctx *context.Context, client *roblox.Client, universeID, newID int64, name string, details *developerproducts.Details, quiesce *assetutils.Quiesce, rateLimitPause time.Duration) error {
	backoff := assetutils.NewRateLimitBackoff(quiesce, rateLimitPause, ctx.Logger, fmt.Sprintf("price for developer product %d", newID))
	_, err := retry.Do(
		retry.NewOptions(retry.Tries(3), retry.Delay(900*time.Millisecond)),
		func(_ int) (struct{}, error) {
			for {
				ctx.PauseController.WaitIfPaused()
				quiesce.Wait()

				err := developerproducts.Update(client, universeID, newID, name, details.Description, details.Price, true)
				if err == nil {
					return struct{}{}, nil
				}
				if errors.Is(err, developerproducts.Errors.ErrOffsale) {
					return struct{}{}, &retry.ExitRetry{Err: err}
				}
				if errors.Is(err, developerproducts.Errors.ErrRateLimited) {
					if !backoff.Wait() {
						return struct{}{}, &retry.ExitRetry{Err: err}
					}
					continue
				}
				return struct{}{}, &retry.ContinueRetry{Err: err}
			}
		},
	)
	return err
}

func classify(ctx *context.Context, r *request.Request, fetch func(int64) (*developerproducts.Details, error), lookedUp *atomic.Int32) ([]*developerproducts.Details, assetutils.SkipReport) {
	var (
		mu     sync.Mutex
		report assetutils.SkipReport
		kept   []*developerproducts.Details
		wg     sync.WaitGroup
	)

	wg.Add(len(r.IDs))
	for _, id := range r.IDs {
		go func(id int64) {
			defer wg.Done()

			details, err := fetch(id)
			lookedUp.Add(1)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				switch {
				case errors.Is(err, developerproducts.Errors.ErrWrongType):
					report.WrongType++
				case errors.Is(err, developerproducts.Errors.ErrNotFound):
					report.NotFound++
				default:
					ctx.Logger.Error(fmt.Sprintf("Failed to look up developer product %d: %v", id, err))
					report.NotFound++
				}
				return
			}
			if newID, ok := ctx.Checkpoint.Get(id); ok {
				ctx.Response.AddItem(response.ResponseItem{OldID: id, NewID: newID})
				report.Resumed++
				return
			}
			kept = append(kept, details)
		}(id)
	}
	wg.Wait()

	return kept, report
}
