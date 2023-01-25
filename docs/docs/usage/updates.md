---
title: "Updating"
---

Hermit is designed in such a way that it and its package manifests are
always on the latest version. To that end, Hermit will check for and upgrade
to new releases of itself once every 24 hours, and will sync to the latest
package definitions every 24 hours. _If you notice a pause when using Hermit,
this is often the cause._

You can read more about the implications of this on package maintenance
in the [packaging introduction](../../packaging/introduction#update-policy).

In addition, some packages may define [channels](../../packaging/schema/channel)
which allow packages to be kept up to date automatically with upstream releases.
Channels specify their own update frequency which Hermit will use to periodically
check for updates. If the ETag for the package has changed, Hermit will download
and upgrade the package.