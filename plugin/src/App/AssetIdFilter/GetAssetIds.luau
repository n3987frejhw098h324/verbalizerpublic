--!strict

export type FilterOptions = {
    WhitelistedInstances: { string },
    Instances: { Instance },
    ItemType: string?,
}

local function isValidId(id: number): boolean
	if id == 0 or id % 1 ~= 0 then return false end

	local idLength = math.floor(math.log10(math.abs(id))) + 1
	return idLength >= 7 and idLength <= 15
end

local function pickBestValidAssetId(str: string): number?
	local best: number? = nil
	local bestLen = 0
	for matched in string.gmatch(str, "%d+") do
		local id = tonumber(matched)
		if id and isValidId(id) then
			local len = #matched
			if len > bestLen or (len == bestLen and best ~= nil and id > best) then
				bestLen = len
				best = id
			end
		end
	end
	return best
end

local function getId(filteredInstance: Instance, possibleId: any): { [number]: { Instance } }
    local id = tonumber(possibleId)
    if not id or not isValidId(id) then return {} end

    return { [id] = { filteredInstance } }
end

local function getStringIds(filteredInstance: Instance, str: string): { [number]: { Instance } }
    local idMap = {}
	for matchedString in string.gmatch(str, "%d+") do
		local id = tonumber(matchedString)
		if not id or idMap[id] or not isValidId(id) then continue end

		idMap[id] = { filteredInstance }
	end
    return idMap
end

local function getAnimationId(animation: Animation): { [number]: { Instance } }
	local s = tostring((animation :: any).AnimationId)
	return getId(animation, pickBestValidAssetId(s))
end

local function getSoundId(sound: Sound): { [number]: { Instance } }
	local s = tostring((sound :: any).SoundId)
	return getId(sound, pickBestValidAssetId(s))
end

local function getScriptIds(scriptInstance: Script | LocalScript | ModuleScript): { [number]: { Instance } }
    return getStringIds(scriptInstance, scriptInstance.Source)
end

local function getStringValueIds(strValue: StringValue): { [number]: { Instance } }
    return getStringIds(strValue, strValue.Value)
end

local function getNumberValueId(numValue: NumberValue | IntValue): { [number]: { Instance } }
    return getId(numValue, numValue.Value)
end

local function getCharacterMesh(characterMesh: CharacterMesh): { [number]: { Instance } }
    return getId(characterMesh, characterMesh.MeshId)
end

local function getMeshPart(meshPart: MeshPart): { [number]: { Instance } }
	local s = tostring((meshPart :: any).MeshId)
	return getId(meshPart, pickBestValidAssetId(s))
end

local function getSpecialMesh(specialMesh: SpecialMesh): { [number]: { Instance } }
	local s = tostring((specialMesh :: any).MeshId)
	return getId(specialMesh, pickBestValidAssetId(s))
end

local function getContentIds(propertyNames: { string }): (instance: any) -> { [number]: { Instance } }
	return function(instance: any): { [number]: { Instance } }
		local idMap: { [number]: { Instance } } = {}
		for _, propertyName in propertyNames do
			local id = pickBestValidAssetId(tostring(instance[propertyName]))
			if id then
				idMap[id] = { instance }
			end
		end
		return idMap
	end
end

local instanceIdGetters: { [string]: (instance: any) -> { [number]: { Instance } } } = {
    Animation = getAnimationId,
    Sound = getSoundId,

    NumberValue = getNumberValueId,
    IntValue = getNumberValueId,
    StringValue = getStringValueIds,

    CharacterMesh = getCharacterMesh,
    MeshPart = getMeshPart,
    SpecialMesh = getSpecialMesh,

    Decal = getContentIds({ "Texture" }),
    Texture = getContentIds({ "Texture" }),
    ImageLabel = getContentIds({ "Image" }),
    ImageButton = getContentIds({ "Image" }),

    ParticleEmitter = getContentIds({ "Texture" }),
    Beam = getContentIds({ "Texture" }),
    Trail = getContentIds({ "Texture" }),

    Sky = getContentIds({ "SkyboxBk", "SkyboxDn", "SkyboxFt", "SkyboxLf", "SkyboxRt", "SkyboxUp", "MoonTextureId", "SunTextureId" }),
    SurfaceAppearance = getContentIds({ "ColorMap", "MetalnessMap", "NormalMap", "RoughnessMap" }),
    MaterialVariant = getContentIds({ "ColorMap", "MetalnessMap", "NormalMap", "RoughnessMap" }),
    ScrollingFrame = getContentIds({ "TopImage", "MidImage", "BottomImage" }),
    ImageHandleAdornment = getContentIds({ "Image" }),
    Tool = getContentIds({ "TextureId" }),
}

local function createFilterMap(instanceFilter: { string }): {[string]: (instance: Instance) -> { [number]: { Instance } }}
    local filterMap = {} :: any
    for _, className in instanceFilter do
        if className == "LuaSourceContainer" then
            filterMap["LocalScript"] = getScriptIds
            filterMap["ModuleScript"] = getScriptIds
            filterMap["Script"] = getScriptIds
        else
            assert(instanceIdGetters[className], `{className} is not a supported instance`)
            filterMap[className] = instanceIdGetters[className]
        end
    end
    return filterMap
end

local function merge(originalIdMap: { [number]: { Instance } }, otherIdMap: { [number]: { Instance } })
    for id, instanceArray in otherIdMap do
        local idInstances = originalIdMap[id]
        if not idInstances then
            originalIdMap[id] = instanceArray
            continue
        end

        for _, instance in instanceArray do
            if table.find(idInstances, instance) then continue end
            table.insert(idInstances :: { any }, instance)
        end
    end
end

local ITEM_ID_PATTERNS: { [string]: { string } } = {
	GamePass = {
		"%f[%w_]PromptGamePassPurchase%s*%(%s*[^,]-,%s*([%w_%.]+)%s*[,%)]",
		"%f[%w_]UserOwnsGamePassAsync%s*%(%s*[^,]-,%s*([%w_%.]+)%s*[,%)]",
		"%f[%w_]GetGamePassProductInfo%s*%(%s*([%w_%.]+)%s*%)",
		"%f[%w_]GetProductInfo%s*%(%s*([%w_%.]+)%s*,%s*[^%)]-InfoType%.GamePass",
		"%f[%w_]GetProductInfo%s*%(%s*([%w_%.]+)%s*,%s*[^%)]-[\"']GamePass[\"']",
	},
	Badge = {
		"%f[%w_]AwardBadge%s*%(%s*[^,]-,%s*([%w_%.]+)%s*[,%)]",
		"%f[%w_]UserHasBadgeAsync%s*%(%s*[^,]-,%s*([%w_%.]+)%s*[,%)]",
		"%f[%w_]GetBadgeInfoAsync%s*%(%s*([%w_%.]+)%s*%)",
	},
	DeveloperProduct = {
		"%f[%w_]PromptProductPurchase%s*%(%s*[^,]-,%s*([%w_%.]+)%s*[,%)]",
		"%f[%w_]GetProductInfo%s*%(%s*([%w_%.]+)%s*,%s*[^%)]-InfoType%.Product",
		"%f[%w_]GetProductInfo%s*%(%s*([%w_%.]+)%s*,%s*[^%)]-[\"']Product[\"']",
	},
}

local function scanItemIds(instances: { Instance }, itemType: string): { [number]: { Instance } }
	local idMap: { [number]: { Instance } } = {}
	local patterns = ITEM_ID_PATTERNS[itemType]
	if not patterns then return idMap end

	local sources: { { instance: Instance, source: string } } = {}
	for _, instance in instances do
		if not instance:IsA("LuaSourceContainer") then continue end
		local source = (instance :: any).Source
		if source and source ~= "" then
			table.insert(sources, { instance = instance :: Instance, source = source :: string })
		end
	end

	local function addId(id: number, instance: Instance)
		if not isValidId(id) then return end
		local instanceArray = idMap[id]
		if not instanceArray then
			idMap[id] = { instance }
		elseif not table.find(instanceArray, instance) then
			table.insert(instanceArray, instance)
		end
	end

	local typedNames: { [string]: boolean } = {}
	for _, entry in sources do
		for _, pattern in patterns do
			for token in string.gmatch(entry.source, pattern) do
				if string.match(token, "^%d+$") then
					local id = tonumber(token)
					if id then addId(id, entry.instance) end
				else
					local name = string.match(token, "([%w_]+)$")
					if name then typedNames[name] = true end
				end
			end
		end
	end

	for name in typedNames do
		local assignPattern = "%f[%w_]" .. name .. "%f[^%w_]%s*=%s*(%d+)"
		local fieldPattern = "%[%s*[\"']" .. name .. "[\"']%s*%]%s*=%s*(%d+)"
		for _, entry in sources do
			for value in string.gmatch(entry.source, assignPattern) do
				local id = tonumber(value)
				if id then addId(id, entry.instance) end
			end
			for value in string.gmatch(entry.source, fieldPattern) do
				local id = tonumber(value)
				if id then addId(id, entry.instance) end
			end
		end
	end

	return idMap
end

return function(filterOptions: FilterOptions): { [number]: { Instance } }
    if filterOptions.ItemType then
        return scanItemIds(filterOptions.Instances, filterOptions.ItemType)
    end

    local idMap = {}
    local filterMap = createFilterMap(filterOptions.WhitelistedInstances)
    for _, instance in filterOptions.Instances do
        local className = instance.ClassName
        local parseId = filterMap[className]
        if not parseId then continue end

        merge(idMap, parseId(instance))
    end
    return idMap
end
