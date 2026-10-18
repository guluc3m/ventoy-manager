# ventoy-updater

An interactive terminal updater for Linux ISOs on a Ventoy drive. It discovers recognised images, finds each distribution's latest published release from its official release index, downloads approved replacements, optionally deletes the old ISO, and refreshes matching `menu_alias` entries in `ventoy/ventoy.json`.


## Build and run

```sh
go build ./cmd/ventoy-updater
ventoy-updater
```

Pass `--ventoy-dir` to specify the drive you intend to update. When the flag is omitted, the updater looks for drives with a `ventoy/ventoy.json` configuration under `/run/media/$USER` and `/media/$USER`.


## Supported images

- Ubuntu Desktop
- Fedora Workstation
- Debian
- Linux Mint Cinnamon
- Tails OS
- Arch Linux
- OpenSUSE Tumbleweed/Leap
- Manjaro KDE

