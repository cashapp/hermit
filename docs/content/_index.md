+++
title = "Hermit"
description = "Hermit manages isolated, self-bootstrapping sets of tools in software projects."
geekdocNav = false
geekdocAlign = "center"
geekdocAnchor = false
+++

### Hermit manages isolated, self-bootstrapping sets of tools in software projects.

{{< columns >}}


Hermit ensures that your team, your contributors, and your CI have the same
consistent tooling.

<--->

Packages installed via Hermit will be available on any future machine, Linux
or Mac, by simply cloning the repository and running the linked binaries.

<--->

Each link will bootstrap Hermit if necessary, then auto-install the package,
before executing the binary.

{{< /columns >}}

{{< button class="get-started" relref="usage/get-started/" >}}Get Started{{< /button >}}

---

{{< columns >}}

## Why Do I Need it?

If you've ever had to add something like the following to your project's README...

> _Make sure you have at least Node 12.x.y, protoc x.y.z, GNU make version 4.x.y, and Go 1.16 or higher._

...then Hermit is for you.

<--->

## Example

{{< asciinema url="screencasts/using.cast" >}}

<--->

## Quickstart
Run this command and follow the instructions:

```text
curl -fsSL https://github.com/cashapp/hermit/releases/download/stable/install.sh | /bin/bash
```

{{< /columns >}}

{{< columns >}}
## Packages

Default packages are available [here](https://github.com/cashapp/hermit-packages).

<--->

## Source code

Contributions are welcome [here](https://github.com/cashapp/hermit).

<--->

## Problems?

Please file an [issue](https://github.com/cashapp/hermit/issues/new) and we'll look into it.

{{< /columns >}}
