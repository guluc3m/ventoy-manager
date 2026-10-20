# ventoy-manager

An interactive terminal manager for Linux ISOs on a Ventoy drive (updating, downloading, and deleting). Also refreshes matching `menu_alias` entries in `ventoy/ventoy.json`, if present.


## Supported images

- [Ubuntu](https://ubuntu.com/) (Desktop LTS, Desktop)
- [Fedora](https://fedoraproject.org/) (Workstation, Silverblue)
- [Rocky Linux](https://rockylinux.org/)
- [Debian](https://www.debian.org/)
- [Linux Mint](https://linuxmint.com/)
- [MX Linux](https://mxlinux.org/)
- [Tails OS](https://tails.net/)
- [Arch Linux](https://archlinux.org/)
- [Artix Linux](https://artixlinux.org/) (Graphical, Community, Base)
- [openSUSE](https://www.opensuse.org/) (Tumbleweed, Leap)
- [Manjaro](https://manjaro.org/)
- [EndeavourOS](https://endeavouros.com/)
- [Nobara](https://nobaraproject.org/) (Official)
- [CachyOS](https://cachyos.org/)
- [Omarchy](https://omarchy.org/)
- [ShredOS](https://github.com/PartialVolume/shredos.x86_64)
- [GParted Live](https://gparted.org/)

Both `amd64` (`x86_64`) and `arm64` (`aarch64`) images are recognized and updated where upstream publishes them: Ubuntu, Fedora, Rocky Linux, Debian and openSUSE Tumbleweed/Leap ship both, while the remaining distributions are `amd64` only.


## Build and run

```sh
go build ./cmd/ventoy-manager
ventoy-manager
```

Pass `--ventoy-dir` to specify the drive you intend to manage. When the flag is omitted, the manager looks for Ventoy drives under `/run/media/$USER` and `/media/$USER`.


### Batch mode

`ventoy-manager --yes` skips the UI: it checks upstream, downloads every available update and deletes the replaced ISOs. `--keep-old` flips that to keep the old files, pre-selecting the keep column.
