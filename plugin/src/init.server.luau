--!strict
local UiLibrary = require("./Plugin/UiLibrary")
local App = require("./Plugin/App")

local RunService = game:GetService("RunService")

if not RunService:IsEdit() then
	return
end

local toolbar = plugin:CreateToolbar("Asset Reuploader")
local widget = plugin:CreateDockWidgetPluginGui(
	"Asset Reuploader",
	DockWidgetPluginGuiInfo.new(Enum.InitialDockState.Float, false, true, 500, 375, 500, 375)
)

local toggleButton = toolbar:CreateButton("Verbalizer", "Original by kartfr // Fork by rod", "rbxassetid://103825301692154")
toggleButton.Click:Connect(function() widget.Enabled = not widget.Enabled end)

widget:GetPropertyChangedSignal("Enabled"):Connect(function() toggleButton:SetActive(widget.Enabled) end)
widget.Title = `Verbalizer`

if plugin:GetSetting("Port") == nil then
	plugin:SetSetting("Port", 38073)
end

local ui = UiLibrary.new(widget, plugin)
App.init(ui, plugin)

plugin.Unloading:Once(function()
	ui:Destroy()
end)
