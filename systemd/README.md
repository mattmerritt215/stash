# Running stash-multiuser as a systemd service

Example files for deploying the fork as a long-running Linux service with a
prebuilt binary (from this repo's GitHub Releases) rather than Docker. See
[`../docker/production`](../docker/production) if you'd rather run it in a
container instead -- these are alternatives, not required together.

- [`stash.service`](stash.service) -- the systemd unit.
- [`stash.env.example`](stash.env.example) -- every `STASH_AUTH_*`/`STASH_OIDC_*`
  variable the auth layer reads, documented with its default. Copy to
  `/etc/stash/env` and edit.

## Why a separate env file at all

Auth config is deliberately kept out of stash's own `config.yml` -- see
[`../FORK.md`](../FORK.md#architecture-decisions) -- so that upstream's config
schema is never touched and secrets (the OIDC client secret, in particular)
never land in the stash data directory alongside your library data and
thumbnails. `STASH_AUTH_MODE` unset, or this file not loaded at all, makes
the binary behave as stock single-user stash.

## First-time setup

```sh
sudo useradd --system --home-dir /var/lib/stash --create-home stash
sudo mkdir -p /opt/stash /etc/stash

# Download the latest stash-linux asset from this repo's Releases page and
# place it at /opt/stash/stash, or build it yourself with `make build-release`.
sudo chmod +x /opt/stash/stash

sudo cp stash.env.example /etc/stash/env
sudo $EDITOR /etc/stash/env            # set STASH_AUTH_MODE and friends
sudo chown root:stash /etc/stash/env
sudo chmod 640 /etc/stash/env

sudo cp stash.service /etc/systemd/system/stash.service
sudo systemctl daemon-reload
sudo systemctl enable --now stash
```

On first start with no `config.yml` at the path named by `STASH_CONFIG_FILE`
in the unit, stash treats it as a new install and walks you through initial
setup the first time you open the web UI (library paths, admin credentials
for the local break-glass login, etc.) -- that part is unchanged from
upstream Stash.

If `stash.service`'s sandboxing directives (`ProtectHome`, `ReadWritePaths`)
don't fit your layout -- for example your media library lives under
`/home` or on a separately-mounted path -- add those paths to
`ReadWritePaths=` or loosen the directive; don't disable the sandbox
wholesale to work around one path.

## Upgrading

```sh
sudo systemctl stop stash
sudo cp stash-linux /opt/stash/stash && sudo chmod +x /opt/stash/stash
sudo systemctl start stash
```

`/etc/stash/env` and the data directory are untouched by a binary swap.
