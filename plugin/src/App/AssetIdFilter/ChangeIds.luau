--!strict
local ApiDump = require("../ApiDump")
local retry = require("../Retry")
local WaitGroup = require("../WaitGroup")

local ScriptEditorService = game:GetService("ScriptEditorService")
local AssetService = game:GetService("AssetService")

local cachedClassProperties: { [string]: { string } } = {}

export type IdPair = {
    newId: number,
    oldId: number
}

local function replaceFullNumberToken(s: string, oldId: number, newId: number): string
	return string.gsub(s, `%f[%d]{oldId}%f[%D]`, tostring(newId))
end

local function setScriptIds(scriptInstance: LuaSourceContainer, idsToChange: { IdPair })
    local source = (scriptInstance :: any).Source
    if not source then
        warn(scriptInstance, "has nil source")
        return
    end

    for _, idPair in idsToChange do
        source = replaceFullNumberToken(source, idPair.oldId, idPair.newId)
    end

    if #source > 200_000 then
        ScriptEditorService:UpdateSourceAsync(scriptInstance, function() return source end)
    else
        (scriptInstance :: any).Source = source
    end
end

local function setAnimationId(animation: Animation, oldId: number, newId: number)
    animation.AnimationId = replaceFullNumberToken(animation.AnimationId, oldId, newId)
end

local function setSoundId(sound: Sound, oldId: number, newId: number)
    sound.SoundId = replaceFullNumberToken(sound.SoundId, oldId, newId)
end

local function setNumberValueId(numValue: NumberValue | IntValue, _: number, newId: number)
    (numValue :: NumberValue).Value = newId
end

local function setStringValueIds(strValue: StringValue, oldId: number, newId: number)
    strValue.Value = replaceFullNumberToken(strValue.Value, oldId, newId)
end

local function setCharacterMesh(characterMesh: CharacterMesh, _: number, newId: number)
    characterMesh.MeshId = newId
end

local function transferAttributes(oldInstance: Instance, newInstance: Instance)
    for name, value in oldInstance:GetAttributes() do
        newInstance:SetAttribute(name, value)
    end
end

local function transferTags(oldInstance: Instance, newInstance: Instance)
    for _, tag in oldInstance:GetTags() do
        newInstance:AddTag(tag)
    end
end

local function transferChildren(oldInstance: Instance, newInstance: Instance)
    for _, child in oldInstance:GetChildren() do
        if child:IsA("TouchTransmitter") then continue end
        child.Parent = newInstance
    end
end

local function transferProperties(oldInstance: Instance, newInstance: Instance)
    local className = oldInstance.ClassName
    if className ~= newInstance.ClassName then error(`oldInstance({className}) class is not equal to newInstance({newInstance.ClassName})`) end
    assert(ApiDump.isCached(), "no api dump cached")

    local cachedProperties = cachedClassProperties[className]
    if not cachedProperties then
        cachedProperties = ApiDump.getProperties(className)
        cachedClassProperties[className] = cachedProperties
    end

    for _, property in cachedProperties do
        if property == "Parent" or property == "Sandboxed" or property == "brickColor" then continue end
        (newInstance :: any)[property] = (oldInstance :: any)[property]
    end
end

local function transferJoints(oldInstance: Instance, newInstance: Instance)
    for _, instance in game:GetDescendants() do
        if not instance:IsA("JointInstance") then continue end
        if instance.Part0 == oldInstance then
            instance.Part0 = newInstance
        elseif instance.Part1 == oldInstance then
            instance.Part1 = newInstance
        end
    end
end

local function setMeshPart(meshPart: MeshPart, oldId: number, newId: number)
    local contentRetrieved, content: Content = retry(3, Content.fromAssetId, newId) 
    if not contentRetrieved then
        warn(`failed to get content from {newId}, skipping {oldId}`)
        return
    end
    
    local newMeshPart = AssetService:CreateMeshPartAsync(content, {
        CollisionFidelity = meshPart.CollisionFidelity,
        RenderFidelity = meshPart.RenderFidelity,
        FluidFidelity = meshPart.FluidFidelity
    } :: any)

    local success, result = pcall(function()
        transferProperties(meshPart, newMeshPart)
        transferAttributes(meshPart, newMeshPart)
        transferTags(meshPart, newMeshPart)
        transferChildren(meshPart, newMeshPart)
        transferJoints(meshPart, newMeshPart)
    end)
    if not success then
        newMeshPart:Destroy()
        error(result)
    end

    newMeshPart.Parent = meshPart.Parent
    meshPart:Destroy()
end

local function setSpecialMesh(specialMesh: SpecialMesh, oldId: number, newId: number)
    specialMesh.MeshId = replaceFullNumberToken(specialMesh.MeshId, oldId, newId)
end

local function setContentIds(propertyNames: { string }): (instance: any, oldId: number, newId: number) -> ()
    return function(instance: any, oldId: number, newId: number)
        for _, propertyName in propertyNames do
            local current = tostring(instance[propertyName])
            local updated = replaceFullNumberToken(current, oldId, newId)
            if updated ~= current then
                instance[propertyName] = updated
            end
        end
    end
end

local instanceIdSetters = {
    Animation = setAnimationId,
    Sound = setSoundId,

    NumberValue = setNumberValueId,
    IntValue = setNumberValueId,
    StringValue = setStringValueIds,

    CharacterMesh = setCharacterMesh,
    MeshPart = setMeshPart,
    SpecialMesh = setSpecialMesh,

    Decal = setContentIds({ "Texture" }),
    Texture = setContentIds({ "Texture" }),
    ImageLabel = setContentIds({ "Image" }),
    ImageButton = setContentIds({ "Image" }),

    ParticleEmitter = setContentIds({ "Texture" }),
    Beam = setContentIds({ "Texture" }),
    Trail = setContentIds({ "Texture" }),

    Sky = setContentIds({ "SkyboxBk", "SkyboxDn", "SkyboxFt", "SkyboxLf", "SkyboxRt", "SkyboxUp", "MoonTextureId", "SunTextureId" }),
    SurfaceAppearance = setContentIds({ "ColorMap", "MetalnessMap", "NormalMap", "RoughnessMap" }),
    MaterialVariant = setContentIds({ "ColorMap", "MetalnessMap", "NormalMap", "RoughnessMap" }),
    ScrollingFrame = setContentIds({ "TopImage", "MidImage", "BottomImage" }),
    ImageHandleAdornment = setContentIds({ "Image" }),
    Tool = setContentIds({ "TextureId" }),
}

return function(filteredIds: { [number]: { Instance } }, idsToChange: { IdPair })
    local waitGroup = WaitGroup.new()
    local scriptIdChanges = {}

    for _, idPair in idsToChange do
        local oldId = idPair.oldId
        local newId = idPair.newId

        local instanceArray = filteredIds[oldId]
        if not instanceArray then continue end

        for _, instance in instanceArray do
            if instance:IsA("LuaSourceContainer") then
                if not scriptIdChanges[instance] then scriptIdChanges[instance] = {} end
                table.insert(scriptIdChanges[instance], idPair)
                continue
            end

            local className = instance.ClassName
            local setInstanceId = instanceIdSetters[className]
            if not setInstanceId then
                warn(`{className} is not supported, skipping`)
                continue
            end

            waitGroup:Add(function()
                local success, result = pcall(setInstanceId :: any, instance, oldId, newId)
                if not success then warn("Failed to change", instance, `{oldId} to {newId}:`, result) end
            end)
        end
    end

    for instance, idPairs in scriptIdChanges do
        waitGroup:Add(function()
            local success, result = pcall(setScriptIds :: any, instance, idPairs)
            if not success then warn("Failed to bulk change", instance, result) end
        end)
    end

    waitGroup:Wait()
end
