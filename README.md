![gocomply logo](gocomply.png)

# gocomply <sup>beta</sup>

Give open source Golang developers the credit they deserve, follow your legal
obligations, and save time with `gocomply`.

This little 500ish-line program scans the Go module in the current
directory for all direct and indirect dependencies, and attempts to download 
and write all of their license files to stdout. Progress or warnings are 
written to stderr.

## Use

Install gocomply (you only need to do this once)

```
$ go install tawesoft.co.uk/gopkg/gocomply@latest
```

Then, go (pun not intended) to the directory of some Go module

```
$ cd path/to/some/module
```

Then just run `gocomply`. You probably want to redirect its output to a file,
like so. This will overwrite that file each time. You'll see some progress
on the terminal.

```
$ gocomply > 3rd-party-licenses.txt
```

## Authentication

Gocomply can improve its accuracy, run faster, and access private 
repositories if it's able to access the necessary APIs. For GitHub, this
requires authentication to avoid being rate limited.

### Get a personal access token

Visit [github.com/settings/tokens](https://github.com/settings/tokens), click
"Generate New Token". If you don't care about private repositories, grant 
(only) the permission `public_repo Access public repositories`. Otherwise,
grant the permission `repo Full control of private repositories`.

### Update your .netrc file

You might already have done this if using private repos with Go.

```
machine github.com login USERNAME password PERSONAL_ACCESS_TOKEN
```

Gocomply looks for this at the default location of `$HOME/.netrc`, or at the 
location specified by the NETRC environment variable.

## Important caveats

A human must manually check the output for compliance. Just because you have
included the text of a license file, it does not mean you're allowed to use
the code or that the license is open source. It does not mean that the
author of the module that you depend on is using the license properly. It
does not mean that there isn't a bug in gocomply. It does not mean that
gocomply was completely accurate in downloading the correct license file.

The tool only checks the currently published version of a license. You might
be using an old version that comes under a different license.

Because `git archive` isn't widely supported (shame!) the method of
obtaining a single license file from a git repo is something that must be
hard-coded for each provider. The provider you use might be missing from
this hard-coded list - if so, open an issue.

The `gocomply` program also operates in a different mode where it accepts a
list of modules to check as command-line arguments. Subtly, it is assumed that
this is a complete list of modules and dependencies - the dependencies of
modules provided on the command-line are NOT checked. This mode is intended for
users who parse the output of `go list -m all` themselves.

Note that the generated `3rd-party-licenses.txt` only applies to any binary
built from or including your source code. If you're just distributing your own 
source code, you're probably not redistributing its source dependencies. The 
programmer *using* your source code is obtaining the dependencies themselves
with `go get`. Only the person ultimately creating binaries is the one
who strictly needs to `gocomply`. Otherwise, don't put other people's licenses
into your license.txt - keep them separate e.g. in `3rd-party-licenses.txt`.

## Troubleshooting

### `panic: error: go list error: exit status 1`

The current directory is not a Go module.

## Feedback

This is early software, so feel free to open an issue or contact a maintainer:

* Ben Golightly <[ben@tawsoft.co.uk](mailto:ben@tawsoft.co.uk)>
