package assetutils

import (
	"net"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/clientutils"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/taskqueue"
)

const AssetsInfoChunkSize int = 50

type AssetsInfoResult = taskqueue.TaskResult[develop.GetAssetsInfoResponse]

func GetAssetsInfoInChunks(ctx *context.Context, r *request.Request, quiesce *Quiesce, pause time.Duration) []chan AssetsInfoResult {
	queue := taskqueue.New[develop.GetAssetsInfoResponse](time.Minute, config.GetInt("assets_info_per_minute"))

	newAssetsInfoHandler := func(ids []int64) func() (develop.GetAssetsInfoResponse, error) {
		return func() (develop.GetAssetsInfoResponse, error) {
			handler, err := develop.NewAssetsInfoHandler(ctx.Client, ids)
			if err != nil {
				return develop.GetAssetsInfoResponse{}, err
			}

			return retry.Do(
				retry.NewOptions(retry.Tries(3)),
				func(try int) (develop.GetAssetsInfoResponse, error) {
					for {
						if ctx.Cancelled() {
							return develop.GetAssetsInfoResponse{}, nil
						}

						ctx.PauseController.WaitIfPaused()
						quiesce.Wait()
						if try > 1 {
							queue.Limiter.Wait()
						}

						assetsInfo, err := handler()
						if err == nil {
							return assetsInfo, nil
						}

						if err == develop.GetAssetsInfoErrors.ErrRateLimited {
							quiesce.Extend(pause)
							continue
						}

						if err == develop.GetAssetsInfoErrors.ErrUnauthorized {
							clientutils.GetNewCookie(ctx, r, ctx.Client, "cookie expired")
						} else {
							switch err.(type) {
							case *net.OpError, *net.DNSError:
								queue.Limiter.Decrement()
							}
						}

						return develop.GetAssetsInfoResponse{}, &retry.ContinueRetry{Err: err}
					}
				},
			)
		}
	}

	ids := r.IDs

	chunkAmount := (len(ids) + AssetsInfoChunkSize - 1) / AssetsInfoChunkSize
	tasks := make([]chan AssetsInfoResult, 0, chunkAmount)
	for start, end := 0, 50; start < len(ids); start, end = start+50, end+50 {
		end = min(end, len(ids))
		idChunk := ids[start:end]
		tasks = append(tasks, queue.QueueTask(newAssetsInfoHandler(idChunk)))
	}

	return tasks
}
