### 💥 Note: This is the first release of **stash-multiuser**, a hard fork of Stash adding OIDC/reverse-proxy authentication, role-based permissions, and per-user personalization on top of stock, single-user Stash. See [FORK.md](https://github.com/mattmerritt215/stash/blob/multiuser/FORK.md) for the architecture and merge-from-upstream process.

### ✨ New Features

* Multi-user authentication via OIDC or a trusted reverse proxy, layered entirely on top of stock Stash without touching its own config schema -- a single-user install upgrades losslessly and keeps working unmodified if the multi-user layer is left disabled.
* Four roles (`guest`, `user`, `curator`, `admin`) gating eight permissions, with roles assigned from OIDC group claims or manually, re-evaluated on every sign-in.
* Per-user personalization: favoriting, rating, "organized", and resume time are now personal to each signed-in user rather than shared library state, with upgrades from a single-user install seeding user 1 with every pre-existing value.
* Added a "Random" scene button and a per-scene "exclude from random" toggle, so a user can curate their own random pool without affecting other users or the library itself.
* Added Continuous Play: a "Start" button on Studio, Performer, and Scene cards that queues a randomly-shuffled, "exclude from random"-aware run through that studio's or performer's scenes, with autoplay and continue-playlist already engaged.
* Added a server-synced light/dark display preference (a navbar sun/moon toggle), personal to each user under the multi-user layer the same way other interface settings already are.

### 🎨 Improvements

* The "Random" button now sits in the main nav row alongside Scenes/Images/Groups/etc., and is togglable from Settings -> Interface -> Menu Items like every other entry, instead of living in the always-visible utility icon strip.
* Moved the Donate button out of the navbar and into Settings -> About, next to the existing Open Collective link.
