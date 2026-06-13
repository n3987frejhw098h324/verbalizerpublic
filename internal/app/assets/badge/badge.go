package badge

import (
	"errors"
	"fmt"
	"net"
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
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/badges"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/thumbnails"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/taskqueue"
)

const (
	createRetryTries = 5
	freeBadgesPerDay = 5
	robuxPerBadge    = 100
)

var errStopped = errors.New("reupload stopped")

func Reupload(ctx *context.Context, r *request.Request) {
	logger := ctx.Logger
	pauseController := ctx.PauseController
	resp := ctx.Response

	total := len(r.IDs)
	var processed atomic.Int32

	var badgeIndex atomic.Int32

	nClients := ctx.Clients.Len()
	rateLimitPause := time.Duration(config.GetInt("rate_limit_pause_seconds")) * time.Second
	quiesce := assetutils.NewQuiesce()
	detailsQueue := taskqueue.New[*badges.Details](time.Minute, config.GetInt("item_details_per_minute")*nClients)
	createQueue := taskqueue.New[int64](time.Minute, config.GetInt("badge_creates_per_minute")*nClients)

	logger.Println("Reuploading badges...")

	reporter := assetutils.NewReporter(logger, total, &processed, resp)

	var lookedUp atomic.Int32
	resp.SetProgress(total, &lookedUp)
	resp.SetCurrent("looking up badges")
	ctx.StartProgress()

	fetchDetails := func(id int64) (*badges.Details, error) {
		backoff := assetutils.NewRateLimitBackoff(quiesce, rateLimitPause, logger, fmt.Sprintf("badge %d", id))
		return retry.Do(
			retry.NewOptions(retry.Tries(3)),
			func(_ int) (*badges.Details, error) {
				for {
					pauseController.WaitIfPaused()
					quiesce.Wait()

					res := <-detailsQueue.QueueTask(func() (*badges.Details, error) {
						return badges.GetDetails(ctx.Clients.Next(), id)
					})
					details, err := res.Result, res.Error
					if err == nil {
						return details, nil
					}
					if errors.Is(err, badges.Errors.ErrRateLimited) {
						if !backoff.Wait() {
							return nil, &retry.ExitRetry{Err: err}
						}
						continue
					}
					if errors.Is(err, badges.Errors.ErrNotFound) {
						return nil, &retry.ExitRetry{Err: err}
					}
					logger.Verbose(fmt.Sprintf("Retrying details lookup for %d: %v", id, err))
					return nil, &retry.ContinueRetry{Err: err}
				}
			},
		)
	}

	kept, report := classify(ctx, r, fetchDetails, &lookedUp)
	ctx.StopProgress()

	resp.SetProgress(total, &processed)
	resp.SetCurrent("")
	processed.Add(int32(report.Total()))
	resp.SetSkipSummary(report.Lines("badges"), report.AlreadyYours+report.WrongType+report.NotFound)
	for _, line := range report.Lines("badges") {
		logger.Println(line)
	}
	if len(kept) == 0 {
		return
	}

	if len(kept) > freeBadgesPerDay && !confirmCost(len(kept)) {
		resp.SetStopped("cancelled before creating badges (declined Robux cost)")
		logger.Warn("Cancelled - no badges were created.")
		return
	}

	ctx.StartProgress()
	defer ctx.StopProgress()

	createOne := func(wg *sync.WaitGroup, details *badges.Details) {
		defer wg.Done()

		if ctx.Cancelled() {
			return
		}

		client := ctx.Clients.Next()
		resp.SetCurrent(details.Name)
		info := &develop.AssetInfo{ID: details.ID, Name: details.Name}

		icon, err := clientutils.FetchIcon(client, func() (string, error) {
			return thumbnails.BadgeIcon(client, details.ID)
		})
		if err != nil {
			reporter.UploadError("Failed to get badge icon", info, err)
			return
		}
		const contentType = "image/png"

		expectedCost := int64(0)
		if badgeIndex.Add(1)-1 >= freeBadgesPerDay {
			expectedCost = robuxPerBadge
		}

		name := details.Name
		backoff := assetutils.NewRateLimitBackoff(quiesce, rateLimitPause, logger, fmt.Sprintf("badge %q", name))
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
						return badges.Create(client, r.UniverseID, name, details.Description, expectedCost, icon, contentType)
					})
					id, err := res.Result, res.Error
					if err == nil {
						return id, nil
					}
					if errors.Is(err, badges.Errors.ErrRateLimited) {
						if !backoff.Wait() {
							return 0, &retry.ExitRetry{Err: err}
						}
						continue
					}
					switch {
					case errors.Is(err, badges.Errors.ErrInsufficientFunds):
						if ctx.Cancel() {
							resp.SetStopped("ran out of Robux for badge creation")
							logger.Error("Ran out of Robux - stopping. Badges already created will still be applied.")
						}
						return 0, &retry.ExitRetry{Err: errStopped}
					case errors.Is(err, badges.Errors.ErrNotAuthenticated):
						clientutils.GetNewCookie(ctx, r, client, "cookie expired")
					case errors.Is(err, badges.Errors.ErrModerated):
						name = fmt.Sprintf("(%s) [Censored]", name)
					case errors.Is(err, badges.Errors.ErrTokenInvalid):
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
			reporter.UploadError("Failed to create badge", info, err)
			assetutils.NoteUploadFailure(ctx, client)
			return
		}
		ctx.Clients.ReportSuccess(client)

		reporter.Success(info, newID)
		resp.AddItem(response.ResponseItem{OldID: details.ID, NewID: newID})
		if err := ctx.Checkpoint.Record(details.ID, newID); err != nil {
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

func confirmCost(toCreate int) bool {
	chargeable := toCreate - freeBadgesPerDay
	cost := chargeable * robuxPerBadge

	fmt.Println()
	color.Warn.Println(fmt.Sprintf("This run will create %d badges.", toCreate))
	fmt.Printf("Roblox gives %d free badges per game every 24h; the other %d cost %d Robux each.\n", freeBadgesPerDay, chargeable, robuxPerBadge)
	color.Warn.Println(fmt.Sprintf("Estimated cost: %d Robux (assuming this game's free allowance is unused today).", cost))
	fmt.Println("Any badges already made for this game today also count against the free 5, so the real cost may be higher.")

	for {
		answer, err := console.Input("Continue and spend Robux on badges? (y/N): ")
		if err != nil {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true
		case "", "n", "no":
			return false
		}
	}
}

func classify(ctx *context.Context, r *request.Request, fetch func(int64) (*badges.Details, error), lookedUp *atomic.Int32) ([]*badges.Details, assetutils.SkipReport) {
	var (
		mu     sync.Mutex
		report assetutils.SkipReport
		kept   []*badges.Details
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
				if !errors.Is(err, badges.Errors.ErrNotFound) {
					ctx.Logger.Error(fmt.Sprintf("Failed to look up badge %d: %v", id, err))
				}
				report.NotFound++
				return
			}
			if newID, ok := ctx.Checkpoint.Get(id); ok {
				ctx.Response.AddItem(response.ResponseItem{OldID: id, NewID: newID})
				report.Resumed++
				return
			}
			if details.AwardingUniverseID != 0 && details.AwardingUniverseID == r.UniverseID {
				report.AlreadyYours++
				return
			}
			kept = append(kept, details)
		}(id)
	}
	wg.Wait()

	return kept, report
}
