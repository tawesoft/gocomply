![gocomply logo](gocomply.png)

# gocomply <sup>beta</sup>

Gocomply helps you save time licensing open source third-party Golang source
code by fetching license information for all direct and indirect dependencies.

gocomply scans the Go module in the current directory for all direct and
indirect dependencies, and attempts to download and write all of their license
files to stdout. Progress or warnings may be written to stderr.

## Usage

Install

```
$ go install tawesoft.co.uk/gopkg/gocomply@latest
```

Get license information from the directory of some Go module

```
$ cd path/to/some/module
$ gocomply > 3rd-party-licenses.txt
```

## Important caveats

Licenses of indirect dependencies will be included, regardless of whether
they end up being used by your project or in the resulting binary. You can
and should review and trim the output as appropriate.

A human must manually check the output for compliance. Just because you have
included the text of a license file, it does not mean you're allowed to use
the code or that the license is open source. It does not mean that the
author of the module that you depend on is using the license properly.

The tool only checks the currently published version of a license. You might
be using an old version that comes under a different license.

The tool doesn't yet support private repos.

Because `git archive` isn't widely supported (shame!) the method of
obtaining a single license file from a git repo is something that must be
hard-coded for each provider. The provider you use might be missing from
this hard-coded list - if so, open an issue.

The `gocomply` program also operates in a different mode where it accepts a
list of modules to check as command-line arguments. Subtly, it is assumed that
this is a complete list of modules and dependencies - the dependencies of
modules provided on the command-line are NOT checked. This mode is intended for
users who parse the output of `go list -m all` themselves.

## Troubleshooting

### `panic: error: go list error: exit status 1`

The current directory is not a Go module.

## Feedback

This is early software, so feel free to open an issue or contact a maintainer:

* Ben Golightly <[ben@tawsoft.co.uk](mailto:ben@tawsoft.co.uk)>
