# ventoy-updater

An interactive terminal updater for Linux ISOs on a Ventoy drive. It discovers recognised images, finds each distribution's latest published release from its official release index (including newer major releases), downloads approved replacements, optionally deletes the old ISO, and refreshes matching `menu_alias` entries in `ventoy/ventoy.json`.

## Install and run

```sh
go install github.com/guluc3m/ventoy-updater/cmd/ventoy-updater@latest
ventoy-updater --ventoy-dir /run/media/$USER/Ventoy/ventoy
```

Omit `--ventoy-dir` to auto-detect the drive; with several Ventoy drives mounted you will be prompted to pick one.

The tool never guesses a writable device: pass `--ventoy-dir` for the drive you intend to update. When the flag is omitted, the updater looks for drives with a `ventoy/ventoy.json` configuration under `/run/media/$USER` and `/media/$USER`; if exactly one is mounted it is used, if none it falls back to `./ventoy`, and if several are mounted it asks which one, showing each drive's total size, free space and ISO count. Scripts must pass `--ventoy-dir` explicitly (or use `--yes`, which refuses to pick between multiple drives). ISO discovery follows `control.VTOY_DEFAULT_SEARCH_ROOT` in that drive's `ventoy.json` (and scans the partition root only when Ventoy has no such setting).

Use space to choose updates, or `a` to select every available update. `D` toggles deletion for every available update; `d` independently toggles deletion for the selected row. Press Enter to start. `--yes` is suitable for scripts; add `--keep-old` to retain originals.

## Supported images

Ubuntu Desktop, Fedora Workstation, Debian DVD, Linux Mint Cinnamon, Tails, Arch Linux, openSUSE Tumbleweed/Leap, and Manjaro KDE filenames are recognised. Sources are official HTTPS directory indexes. Unknown images are left untouched; a source failure is shown in the interface and never results in deletion.

Aliases are formed consistently from the distro, edition, version and (when present) flavour—for example `Ubuntu Desktop LTS 26.04.1` and `Linux Mint 22.3 Cinnamon`.

Downloads are streamed to a temporary file in the ISO directory and atomically renamed only when complete. `ventoy.json` is written atomically only after all selected operations finish.
