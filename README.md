# Verbalizer

A simple fork of [kartFr's Asset-Reuploader](https://github.com/kartFr/Asset-Reuploader), designed to be optimized, more beginner-friendly and handle edge cases better. It finds the mesh, sound, and animation IDs in a place,
reuploads them to the place owner, and rewrites the IDs to point at the new copies.

Verbalizer has two parts: a local server (the `.exe`) and a Studio plugin (the `.rbxm`). The plugin connects to the server while it's running to do the reuploading, so keep the `.exe` open the whole time.

## Install

**Prebuilt:** grab the executable from [Releases](../../releases). For the plugin, use the `.rbxm` inside [Releases](../../releases).

**From source:** (building the plugin requires [Rojo](https://rojo.space))

```sh
go build -ldflags "-s -w" -o build/verbalizer.exe ./cmd/assetreuploader

rojo build plugin/default.project.json -o build/verbalizer.rbxm
```

## Usage

1. Run `verbalizer.exe` and follow the instructions on your screen. Leave it running.
2. In Studio, right click Workspace, then press "Import from file...". After that, select `verbalizer.rbxm` and click "Open".
3. Right click the new "Verbalizer" folder, then click on "Save as Local Plugin...".
4. At the top of your Roblox Studio, press on the "Plugins" tab, then click on "Verbalizer".
5. Select the type of assets you want to reupload, then press the "Reupload" button and wait for the server to finish!

## Note
Reuploaded assets are owned by whoever owns the place you're working in. Your account if it's a personal place, or the group if it's a group place. They are uploaded to the account you used for the the `.exe`, not the account you run the plugin on.

## Disclaimer

1. Provided as-is for accounts and content you control. You're responsible for
complying with Roblox's Terms of Service. Not responsible for bans or lost assets.
2. Perfectly reuploading all sounds from a file is nearly impossible due to Roblox permissions.
3. This tool is not perfect. If any crash or unexplained error occurs, please report it to me.
4. In most cases, I do not help people to use this tool. However, I'll try my best to help you all whenever I'm free!
5. Roblox might moderate you falsely for the assets you reupload. In most cases, you can just appeal it and theyll accept it.
