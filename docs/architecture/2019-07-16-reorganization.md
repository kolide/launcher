# Project Reorganization

## Authors

- Logan McPhail ([@loganmac](https://github.com/loganmac))

## Status

- Proposed (July 16, 2018)

## Context

Launcher was an early service developed before current project organization best-practices were decided at Kolide. This proposal details the standardization of the project's package organization in a way that is consistent with Go projects at Kolide.

This opinionated project layout is an effort to both ease the barrier to entry, and provide clear guidelines for new and infrequent contributors to the project.

## Proposed Decision

Packages that are compiled into binaries remain under their respective `/cmd/binary_name` folders.

Packages containing source code that are consumed by packages that create binaries ("main" packages) are organized under `/pkg`.

Source code that was more analogous to a script that would be run, or some minor tooling was left untouched. For example, `/tools/download-osquery.go`.

This subjectively allows for a clean separation of source code for libraries, source code for applications/binaries, and tooling/documentation files and folders.

This also pulls nested packages (for example, `osquery/table` and `osquery/runtime`) into the pkg folder, and names them according to [Go package naming best practices](https://www.google.com/search?q=naming+go+packages&oq=naming+go+packages&aqs=chrome..69i57.2262j0j7&sourceid=chrome&ie=UTF-8).

## Consequences

This should hopefully provide some clarity around where source code for different use-cases belongs to newcomers and infrequent contributors to the project.

No changes to the internals of the packages have been made, except to adjust for new paths to package folders.

Other projects that rely on paths in this project need to be updated to the new locations of libraries.