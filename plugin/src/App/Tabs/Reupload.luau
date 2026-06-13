--!strict
local getFilterOptions = require("../GetFilterOptions")
local reuploadIds = require("../ReuploadIds")
local UiLibrary = require("../../UiLibrary")
local ApiDump = require("../ApiDump")

local Selection = game:GetService("Selection")

type AssetType = "Animation" | "Sound" | "Mesh" | "Decal" | "GamePass" | "DeveloperProduct" | "Badge"

local ASSET_TYPES: { AssetType } = { "Animation", "Sound", "Mesh", "Decal", "GamePass", "DeveloperProduct", "Badge" }

local meshSettings: { [string]: string } = {
	MeshParts = "MeshPart",
	SpecialMeshes = "SpecialMesh",
	CharacterMeshes = "CharacterMesh",
}

local DECAL_SCAN_CLASSES: { string } = { "Decal", "Texture", "ImageLabel", "ImageButton", "ParticleEmitter", "Beam", "Trail", "Sky", "SurfaceAppearance", "MaterialVariant", "ScrollingFrame", "ImageHandleAdornment", "Tool" }

local ITEM_SCAN_CLASSES: { string } = { "LuaSourceContainer" }

local ITEM_TYPES: { [string]: boolean } = {
	GamePass = true,
	DeveloperProduct = true,
	Badge = true,
}

local SELECTED_WHITELIST: { [string]: { string } } = {
	Animation = { "Animation", "StringValue", "NumberValue", "IntValue", "LuaSourceContainer" },
	Sound = { "Sound", "StringValue", "NumberValue", "IntValue", "LuaSourceContainer" },
	Mesh = { "MeshPart", "SpecialMesh", "CharacterMesh", "StringValue", "NumberValue", "IntValue", "LuaSourceContainer" },
	Decal = { "Decal", "Texture", "ImageLabel", "ImageButton", "ParticleEmitter", "Beam", "Trail", "Sky", "SurfaceAppearance", "MaterialVariant", "ScrollingFrame", "ImageHandleAdornment", "Tool", "StringValue", "NumberValue", "IntValue", "LuaSourceContainer" },
	GamePass = ITEM_SCAN_CLASSES,
	DeveloperProduct = ITEM_SCAN_CLASSES,
	Badge = ITEM_SCAN_CLASSES,
}

return function(ui: UiLibrary.Ui, plugin: Plugin)
	local tab = ui:CreateTab("Reupload")

	task.spawn(ApiDump.get)

	local layoutOrder = 0
	local function place<T>(element: T): T
		layoutOrder += 1
		local instance = (element :: any).instance
		instance.LayoutOrder = layoutOrder
		return element
	end

	local selectedType: AssetType = "Animation"

	local typeToggles: { [string]: any } = {}
	local applyingType = false
	local function setType(newType: AssetType)
		selectedType = newType
		applyingType = true
		for assetType, toggle in typeToggles do
			local shouldBeOn = assetType == newType
			if toggle.state ~= shouldBeOn then
				toggle:Toggle()
			end
		end
		applyingType = false
	end

	place(tab:CreateLabel("Asset type"))
	for _, assetType in ASSET_TYPES do
		typeToggles[assetType] = place(tab:CreateToggle(assetType, function()
			if applyingType then return end
			setType(assetType :: AssetType)
		end))
	end
	setType("Animation")

	place(tab:CreateLabel("Mesh options"))
	local meshSplit = place(tab:CreateSplit())
	meshSplit:CreateToggle("Search mesh parts", function(state)
		plugin:SetSetting("MeshParts", state)
	end, plugin:GetSetting("MeshParts"))
	meshSplit:CreateToggle("Search special meshes", function(state)
		plugin:SetSetting("SpecialMeshes", state)
	end, plugin:GetSetting("SpecialMeshes"))
	local meshSplit2 = place(tab:CreateSplit())
	meshSplit2:CreateToggle("Search character meshes", function(state)
		plugin:SetSetting("CharacterMeshes", state)
	end, plugin:GetSetting("CharacterMeshes"))

	local function meshScanClasses(): { string }
		local classes = {}
		for settingName, className in meshSettings do
			if plugin:GetSetting(settingName) then
				table.insert(classes, className)
			end
		end
		return classes
	end

	local function ensureMeshReady(): boolean
		if ApiDump.isCached() or ApiDump.get() then
			return true
		end
		ui:Notify("Notification", "Failed to get API dump (needed for mesh reupload).")
		return false
	end

	place(tab:CreateButton("Reupload", function()
		if selectedType == "Mesh" and not ensureMeshReady() then return end

		local filter = getFilterOptions(plugin, game:GetDescendants())
		if selectedType == "Mesh" then
			for _, className in meshScanClasses() do
				table.insert(filter.WhitelistedInstances, className)
			end
		elseif selectedType == "Decal" then
			for _, className in DECAL_SCAN_CLASSES do
				table.insert(filter.WhitelistedInstances, className)
			end
		elseif ITEM_TYPES[selectedType] then
			filter.ItemType = selectedType
		else
			table.insert(filter.WhitelistedInstances, selectedType)
		end

		reuploadIds(plugin, ui, filter, selectedType, _G.getPlaceList())
	end))

	place(tab:CreateButton("Reupload Selected", function()
		if selectedType == "Mesh" and not ensureMeshReady() then return end

		local filter = {
			WhitelistedInstances = table.clone(SELECTED_WHITELIST[selectedType]),
			Instances = Selection:Get(),
			ItemType = if ITEM_TYPES[selectedType] then selectedType else nil,
		}
		reuploadIds(plugin, ui, filter, selectedType, _G.getPlaceList())
	end))
end
