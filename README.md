# ventoy-manager

An interactive terminal manager for Linux ISOs on a Ventoy drive (updating, downloading, and deleting). Also refreshes matching `menu_alias` entries in `ventoy/ventoy.json`, if present.


## Supported images

- [Ubuntu Desktop LTS](https://ubuntu.com/)
- [Fedora Workstation](https://fedoraproject.org/workstation/)
- [Fedora KDE Desktop](https://fedoraproject.org/kde/)
- [Fedora Silverblue](https://fedoraproject.org/atomic-desktops/silverblue/)
- [Rocky Linux DVD](https://rockylinux.org/)
- [Debian DVD](https://www.debian.org/)
- [Linux Mint Cinnamon](https://linuxmint.com/)
- [MX Linux XFCE](https://mxlinux.org/)
- [Tails OS](https://tails.net/)
- [Arch Linux](https://archlinux.org/)
- [Artix Linux Base (OpenRC)](https://artixlinux.org/)
- [openSUSE Tumbleweed/Leap](https://www.opensuse.org/)
- [Manjaro KDE](https://manjaro.org/)
- [Manjaro XFCE](https://manjaro.org/)
- [Manjaro GNOME](https://manjaro.org/)
- [EndeavourOS](https://endeavouros.com/)
- [Nobara Official](https://nobaraproject.org/)
- [CachyOS Desktop](https://cachyos.org/)
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
