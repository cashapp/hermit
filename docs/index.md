---
Summary: Hermit manages isolated, self-bootstrapping sets of tools in software projects.
hide:
  - navigation
  - toc
---

# Hermit
### Hermit manages isolated, self-bootstrapping sets of tools in software projects.
Hermit ensures that your team, your contributors, and your CI have the same
consistent tooling.

Packages installed via Hermit will be available on any future machine, Linux
or Mac, by simply cloning the repository and running the linked binaries.

Each link will bootstrap Hermit if necessary, then auto-install the package,
before executing the binary.

[Get Started](usage/get-started/){ .md-button }


---


## Why Do I Need it?

If you've ever had to add something like the following to your project's README...

> _Make sure you have at least Node 12.x.y, protoc x.y.z, GNU make version 4.x.y, and Go 1.16 or higher._

...then Hermit is for you.


## Example

<div id="hermit-demo" style="z-index: 1; position: relative; max-width: 60%;"></div>
<script>
  window.onload = function(){
    AsciinemaPlayer.create('static/screencasts/using.cast', document.getElementById('hermit-demo'),{autoplay: true});
}
</script>


## Quickstart
Run this command and follow the instructions:

```bash
curl -fsSL https://github.com/cashapp/hermit/releases/download/stable/install.sh | /bin/bash
```

## Packages

Default packages are available [here](https://github.com/cashapp/hermit-packages).


## Source code

Contributions are welcome [here](https://github.com/cashapp/hermit).


## Problems?

Please file an [issue](https://github.com/cashapp/hermit/issues/new) and we'll look into it.
