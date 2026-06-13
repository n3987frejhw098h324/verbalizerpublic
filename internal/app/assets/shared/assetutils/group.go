package assetutils

import "github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"

func GroupByCreator(assetsInfo []*develop.AssetInfo) map[string]map[int64][]*develop.AssetInfo {
	grouped := make(map[string]map[int64][]*develop.AssetInfo)
	for _, assetInfo := range assetsInfo {
		creatorType := assetInfo.Creator.Type
		creatorID := assetInfo.Creator.TargetID

		byCreatorID, exists := grouped[creatorType]
		if !exists {
			byCreatorID = make(map[int64][]*develop.AssetInfo)
			grouped[creatorType] = byCreatorID
		}
		byCreatorID[creatorID] = append(byCreatorID[creatorID], assetInfo)
	}
	return grouped
}
