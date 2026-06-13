package assetutils

import (
	"fmt"
	"sync/atomic"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/uploaderror"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
)

type progressLogger interface {
	Error(a ...any)
	Success(a ...any)
	Verbose(a ...any)
}

type outcomeSink interface {
	AddFailed(n int)
}

type Reporter struct {
	logger       progressLogger
	total        int
	processed    *atomic.Int32
	sink         outcomeSink
	printSuccess bool
	verbose      bool
}

func NewReporter(logger progressLogger, total int, processed *atomic.Int32, sink outcomeSink) *Reporter {
	return &Reporter{
		logger:       logger,
		total:        total,
		processed:    processed,
		sink:         sink,
		printSuccess: config.GetBool("print_successful_reuploads"),
		verbose:      config.GetBool("verbose"),
	}
}

func (r *Reporter) BatchError(amt int, msg string, err any) {
	end := int(r.processed.Add(int32(amt)))
	start := end - amt
	if r.sink != nil {
		r.sink.AddFailed(amt)
	}
	r.logger.Error(uploaderror.NewBatch(start, end, r.total, msg, err))
}

func (r *Reporter) UploadError(msg string, assetInfo *develop.AssetInfo, err any) {
	processed := r.processed.Add(1)
	if r.sink != nil {
		r.sink.AddFailed(1)
	}
	r.logger.Error(uploaderror.New(int(processed), r.total, msg, assetInfo, err))
}

func (r *Reporter) Retry(assetInfo *develop.AssetInfo, err any) {
	if !r.verbose {
		return
	}
	r.logger.Verbose(fmt.Sprintf(">> retrying %s(%d): %v", assetInfo.Name, assetInfo.ID, err))
}

func (r *Reporter) Success(assetInfo *develop.AssetInfo, newID int64) {
	processed := r.processed.Add(1)
	if r.printSuccess {
		r.logger.Success(uploaderror.New(int(processed), r.total, "", assetInfo, newID))
		return
	}
	if r.verbose {
		r.logger.Verbose(uploaderror.New(int(processed), r.total, "", assetInfo, newID))
	}
}
