+++
title = "Hermit"
+++


{{< block "grid-3" >}}
{{< column >}}

# Hermit

Hermit installs self-bootstrapping tools for software projects in
self-contained, isolated sets, so your team, your contributors, and your CI
have the same consistent tooling.
<a href="/docs/usage/screencast/">
<video width="100%" autoplay playsinline loop>
	  <source src="/images/hermit.mp4" type="video/mp4">
</video>
</a>

{{< button "docs/usage/introduction/" "Get Started" >}}{{< button "docs/faq/" "FAQ" >}}{{< button "docs/packaging/" "Packaging" >}}{{< button "https://github.com/cashapp/hermit-packages" "Packages" >}}
{{< /column >}}

{{< column >}}
# Why Do I Need it?

If you've ever had to add something like the following to your project's README...

> Make sure you have at least Node 12.x.y, protoc x.y.z, GNU make version 4.x.y, and Go 1.16 or higher.

...then Hermit is for you...

```text
$ pwd
/home/user/project
$ hermit init
🐚 $ hermit install make protoc-3.7.1 go-1.16.3 node-12.18.3
🐚 $ which make protoc go node
/home/user/project/bin/make
/home/user/project/bin/protoc
/home/user/project/bin/go
/home/user/project/bin/node
🐚 $ go version
go version go1.16.3 darwin/amd64
🐚 $ node --version
v12.18.3
🐚 $ protoc --version
libprotoc 3.7.1
🐚 $ make --version
GNU Make 4.2.1
```

These packages will now be available on any future machine, Linux or Mac, by
simply cloning the repository and running the linked binaries. Each link will
bootstrap Hermit if necessary, then auto-install the package, before
executing the binary.

{{< /column >}}

{{< column >}}
# Quickstart
Run this command and follow the instructions:

```text
curl -fsSL https://github.com/cashapp/hermit/releases/download/stable/install.sh | /bin/bash
```

Or read the [introduction](docs/usage/introduction/) for more detailed instructions.

{{< figure src="/images/logo.svg" alt="Hermit Crab Vectors by Vecteezy" attr="Hermit Crab Vectors by Vecteezy" attrlink="https://www.vecteezy.com/free-vector/hermit-crab" >}}

{{< /column >}}
{{< /block >}}
