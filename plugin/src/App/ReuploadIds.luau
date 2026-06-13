--!strict
local AssetIdFilter = require("./AssetIdFilter")
local Connection = require("./Connection")
local retry = require("./Retry")
local StatusCodes = require("./StatusCodes")
local UiLibrary = require("../UiLibrary")

local MarketplaceService = game:GetService("MarketplaceService")
local ScriptEditorService = game:GetService("ScriptEditorService")
local HttpService = game:GetService("HttpService")

local currentVersion = script.Parent.Parent.Parent.Version.Value
local busy = false

type ProductInfo = {
    AssetTypeId: number,
    TargetId: number,
    IsNew: boolean,
    Updated: string,
    AssetId: number,
    ProductId: number,
    MinimumMembershipLevel: number,
    Created: string,
    Creator: {
        Name: string,
        CreatorTargetId: number,
        Id: number,
        HasVerifiedBadge: boolean,
        CreatorType: string
    },
    IsLimited: boolean,
    ContentRatingTypeId: number,
    Sales: number,
    IsPublicDomain: boolean,
    Name: string,
    IsForSale: boolean,
    Description: string,
    IconImageAssetId: number,
    IsLimitedUnique: boolean
}

type Request = {
	placeId: number,         
	creatorId: number,       
	ids: { number },             
	defaultPlaceIds: { number }, 
	pluginVersion: string,   
	assetType: string,       
	exportJson: boolean,     
	isGroup: boolean     
}

local function scriptInjectionEnabled()
    local testScript = Instance.new("Script")
	local success = pcall(ScriptEditorService.UpdateSourceAsync, ScriptEditorService, testScript, function() 
		return "" 
	end)
	
	testScript:Destroy()
	return success
end

local function getGameInfo(): (boolean, number, number)
    local creatorId, placeId = game.CreatorId, game.PlaceId

    if placeId == 0 then
        return false, creatorId, placeId 
    end

    if creatorId ~= 0 then
        local isGroup =  game.CreatorType == Enum.CreatorType.Group
        return isGroup, creatorId, placeId
    end

    local success, result = retry(3, MarketplaceService.GetProductInfo, MarketplaceService, placeId)
    if not success then
        return false, creatorId, placeId
    end

    local creatorInfo = (result :: ProductInfo).Creator
    local isGroup = creatorInfo.CreatorType == "Group"
    return isGroup, creatorInfo.CreatorTargetId, placeId
end

return function(plugin: Plugin, ui: UiLibrary.Ui, filterOptions: AssetIdFilter.FilterOptions, assetType: "Animation" | "Sound" | "Mesh" | "Decal" | "GamePass" | "DeveloperProduct" | "Badge", placeArray: { number }?)
    if busy then
        ui:Notify("Notification", "Plugin is busy.")
        return 
    end
    busy = true

    if not scriptInjectionEnabled() then
        ui:Notify("Notification", "Plugin needs script injection permission.")
        busy = false
        return
    end

    local isGroup, creatorId, placeId = getGameInfo()
    if placeId == 0 then
        ui:Notify("Notification", "Game needs to be published. Restart place after publishing.")
        busy = false
        return
    elseif creatorId == 0 then
        ui:Notify("Notification", "Failed to get place info. Try again or restart the place.")
        busy = false
        return
    end
    
    local filteredIds = AssetIdFilter.filterInstances(filterOptions)
    if next(filteredIds) == nil then
        ui:Notify("Notification", "No ids found.")
        busy = false
        return
    end

    local connection, response = Connection.new(plugin:GetSetting("Port"))
    if not connection then
        ui:Notify("Notification", "Unable to connect to localhost. Did you start the executable?" )
        busy = false
        return
    end

    local idArray = AssetIdFilter.getIdArray(filteredIds)
    local totalIds = #idArray
    local replacedCount = 0
    local assetLabel = "animations"
    if assetType == "Sound" then
        assetLabel = "sounds"
    elseif assetType == "Mesh" then
        assetLabel = "meshes"
    elseif assetType == "Decal" then
        assetLabel = "decals"
    elseif assetType == "GamePass" then
        assetLabel = "game passes"
    elseif assetType == "DeveloperProduct" then
        assetLabel = "developer products"
    elseif assetType == "Badge" then
        assetLabel = "badges"
    end

    local function onDataRecieved(data: string)
        local success, result = pcall(HttpService.JSONDecode, HttpService, data)
        if not success then
            if data ~= "done" then return end
            
            ui:Notify("Notification", `Done - replaced {replacedCount} out of {totalIds} {assetLabel}.`)
            busy = false
            connection:Destroy()
            return
        end

        local batch = result :: any
        if #batch == 0 then return end
        AssetIdFilter.replaceIds(filteredIds, batch)
        replacedCount += #batch
        warn(`Replaced {replacedCount} out of {totalIds} {assetLabel}`)
    end

    connection.onDataRecieved = onDataRecieved
    connection.onDisconnect = function()
        ui:Notify("Notification", "Disconnected from localhost. (Try reconnecting)")
        busy = false
    end

    local request: Request = {
        pluginVersion = currentVersion,
        creatorId = creatorId,
        isGroup = isGroup,
        assetType = assetType,
        ids = idArray,
        placeId = placeId,
        defaultPlaceIds = placeArray or {},
        exportJson = plugin:GetSetting("ExportJSON")
    }
    
    local success, statusCode = connection:Send("/reupload", HttpService:JSONEncode(request))
    if not success then
        local message: string = (StatusCodes :: any)[statusCode] or ("Unable to send data to localhost.\n" .. statusCode)
        ui:Notify("Notification", message)

        busy = false
        connection:Destroy()
        return
    end

    ui:Notify("Notification", `Sent {#idArray} ids to localhost.`)

    if response then
        onDataRecieved(response)
    end
end
