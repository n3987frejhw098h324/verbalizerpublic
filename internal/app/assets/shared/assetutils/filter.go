package assetutils

import (
	"fmt"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/response"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
)

type SkipReport struct {
	AlreadyYours int
	WrongType    int
	Resumed      int
	NotFound     int
}

func (s SkipReport) Total() int {
	return s.AlreadyYours + s.WrongType + s.Resumed + s.NotFound
}

func (s SkipReport) Lines(categoryNoun string) []string {
	total := s.Total()
	if total == 0 {
		return nil
	}

	lines := []string{fmt.Sprintf("%d skipped:", total)}
	if s.AlreadyYours > 0 {
		lines = append(lines, fmt.Sprintf("  %d already yours", s.AlreadyYours))
	}
	if s.WrongType > 0 {
		lines = append(lines, fmt.Sprintf("  %d are not %s", s.WrongType, categoryNoun))
	}
	if s.Resumed > 0 {
		lines = append(lines, fmt.Sprintf("  %d already reuploaded in a previous run", s.Resumed))
	}
	if s.NotFound > 0 {
		lines = append(lines, fmt.Sprintf("  %d could not be found (deleted, private, or wrong id)", s.NotFound))
	}
	return lines
}

func Classify(ctx *context.Context, r *request.Request, quiesce *Quiesce, pause time.Duration, categoryNoun string, allowedTypes ...int32) ([]*develop.AssetInfo, SkipReport) {
	allowed := make(map[int32]struct{}, len(allowedTypes))
	for _, t := range allowedTypes {
		allowed[t] = struct{}{}
	}

	creatorID := r.CreatorID
	userID := ctx.Client.UserInfo.ID
	checkUserID := !r.IsGroup

	seen := make(map[int64]struct{}, len(r.IDs))
	kept := make([]*develop.AssetInfo, 0, len(r.IDs))
	var report SkipReport

	ctx.Logger.Verbose(fmt.Sprintf("Verifying asset types for %d requested %s id(s)...", len(r.IDs), categoryNoun))

	tasks := GetAssetsInfoInChunks(ctx, r, quiesce, pause)
	for _, task := range tasks {
		res := <-task
		if res.Error != nil {
			ctx.Logger.Error("Failed to verify a batch of asset types: ", res.Error)
			continue
		}

		for _, info := range res.Result.Data {
			if info == nil {
				continue
			}
			seen[info.ID] = struct{}{}

			if _, ok := allowed[info.TypeID]; !ok {
				report.WrongType++
				continue
			}

			assetCreatorID := info.Creator.TargetID
			if assetCreatorID == creatorID || assetCreatorID == 1 || (checkUserID && assetCreatorID == userID) {
				report.AlreadyYours++
				continue
			}

			if newID, ok := ctx.Checkpoint.Get(info.ID); ok {
				ctx.Response.AddItem(response.ResponseItem{OldID: info.ID, NewID: newID})
				report.Resumed++
				continue
			}

			kept = append(kept, info)
		}
	}

	for _, id := range r.IDs {
		if _, ok := seen[id]; !ok {
			report.NotFound++
		}
	}

	ctx.Logger.Verbose(fmt.Sprintf("Classified %d id(s): %d to reupload, %d already yours, %d not %s, %d already done, %d not found",
		len(r.IDs), len(kept), report.AlreadyYours, report.WrongType, categoryNoun, report.Resumed, report.NotFound))

	return kept, report
}

func ChunkAssets(assets []*develop.AssetInfo, size int) [][]*develop.AssetInfo {
	if size <= 0 {
		size = AssetsInfoChunkSize
	}
	chunks := make([][]*develop.AssetInfo, 0, (len(assets)+size-1)/size)
	for start := 0; start < len(assets); start += size {
		end := min(start+size, len(assets))
		chunks = append(chunks, assets[start:end])
	}
	return chunks
}
