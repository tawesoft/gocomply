![gocomply logo](gocomply.png)

# gocomply <sup>beta</sup>

Gocomply helps you save time licensing open source third-party Golang source
code by fetching license information for all direct and dependencies.

gocomply scans the Go module in the current directory for all direct and
indirect dependencies (or the list of Go modules passed as command line
arguments), and attempts to download and write all of their license files to
stdout. Progress or warnings may be written to stderr.

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

## Feedback

This is early software, so feel free to open an issue or contact a maintainer:

* Ben Golightly <[ben@tawsoft.co.uk](mailto:ben@tawsoft.co.uk)>
