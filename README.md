# ventoy-manager

An interactive terminal manager for Linux ISOs on a Ventoy drive (updating, downloading, and deleting). Also refreshes matching `menu_alias` entries in `ventoy/ventoy.json`.


## Supported images

- Ubuntu Desktop LTS
- Fedora Workstation
- Debian DVD
- Linux Mint Cinnamon
- Tails OS
- Arch Linux
- openSUSE Tumbleweed/Leap
- Manjaro KDE


## Build and run

```sh
go build ./cmd/ventoy-manager
ventoy-manager
```

Pass `--ventoy-dir` to specify the drive you intend to manage. When the flag is omitted, the manager looks for drives with a `ventoy` directory under `/run/media/$USER` and `/media/$USER`. A `ventoy/ventoy.json` configuration is optional: without one, ISOs are still managed normally and only the alias refresh is skipped.


### Batch mode

`ventoy-manager --yes` skips the UI: it checks upstream, downloads every available update and deletes the replaced ISOs. `--keep-old` flips that to keep the old files, pre-selecting the keep column.
