# Launcher flag parsing

## Context

### Background

Launcher supports running (approximately) 12 different subcommands; if a subcommand is not provided, it falls back to `runLauncher`. The command-line flags supported by these different subcommands and by `runLauncher` have some overlap, but not complete overlap. There are many flags that are relevant to only one subcommand, or are only relevant to `runLauncher`.

We have handled this in a couple different ways:

#### Category 1

Use `launcher.ParseOptions` only; if new flags are required, add them to `launcher.ParseOptions` and add their corresponding value to the `launcher.Options` struct. An example of adding new flags is the `compactdb` command, which added a `CompactDbMaxTx` flag to `launcher.Options` that would only be relevant for this subcommand. An example of using existing flags is the `doctor` command.

Subcommands in this category:

* `doctor`
* `compactdb`
* `interactive`
* Running launcher itself: `svc`, `svc-fg`, falling back to `runLauncher`

#### Category 2

Create an entirely new flagset within the new subcommand that includes the `config` flag, parse that flagset, and use the resulting `config` flag to call `launcher.ParseOptions`. An example of this is the `flare` subcommand, which has several flags not supported by `launcher.ParseOptions` -- so it parses its own flags, and then calls `launcher.ParseOptions` with only the config flag: `launcher.ParseOptions("flare", []string{"-config", *flConfigFilePath})`.

Subcommands in this category:

* `flare`
* `watchdog`

#### Category 3

Create an entirely new flagset and parse it; do not use `launcher.ParseOptions`. An example of this is the `query-windowsupdates` subcommand, whose flags do not have any overlap with `launcher.Options`.

Subcommands in this category:

* `desktop`
* `download-osquery`
* `uninstall`
* `query-windowsupdates`

Note that the options used for `uninstall` do actually have overlap with `launcher.Options`, unlike the other three.

#### Category 4

Do not parse any flags at all. An example is the `version` subcommand, which does not require any flags (beyond what's used to autoupdate to the latest launcher version)

Subcommands in this category:

* `version`

#### Further considerations

`launcher.ParseOptions` has a fairly significant amount of logic in it to set or update various options, including some pretty important ones, like the root directory. Therefore, whenever we want to parse options that include the options in `launcher.Options`, we want to use `launcher.ParseOptions`, or somehow extract the relevant logic from that function.

When considering any updates to our flag parsing system, we must consider the fact that flag parsing will fail if the flagset receives any unknown flags. (See [launcher #1513](https://github.com/kolide/launcher/issues/1513) for more details.) This means that we cannot pass new unknown flags to `launcher.ParseOptions`, and we additionally cannot delete any flags previously added to `launcher.ParseOptions` (because an old launcher bootstrapping itself to the latest version may have old flags). We handle flag deletion instead by deprecating flags within `launcher.ParseOptions`:

```golang
_ = flagset.String("debug_log_file", "", "DEPRECATED")
```

### Issue

When adding a new subcommand with options that have some but not complete overlap with `launcher.Options`, what do we do?

Category 4 is not suitable, since in this scenario we require command-line options.

Category 3 is probably not suitable, because we likely want to use `launcher.ParseOptions` for any of the options that have overlap. However, we _could_ choose category 3 if we extracted relevant logic from `launcher.ParseOptions` and exported it for use when parsing shared flags.

Category 2 allows us the flexibility of adding new flags without polluting the main `launcher.Options`. However, in practice, it's a more rigid solution than it appears to be. We can only call `launcher.ParseOptions` with flags that are known to it, otherwise parsing will fail, as noted above. Therefore, we have to specifically list all flags that we want to parse in `launcher.ParseOptions`. This works well for the current usecase of `flare` and `watchdog`, since the only flag they want to parse is `config`. However, for new subcommands that may want `config`, plus `root_directory`, plus `debug`, plus several others, this option becomes slightly cumbersome.

We can also consider variations on category 2, where we define a new flagset in the subcommand, but can expose and then pull in in some parts of `launcher.ParseOptions` as necessary. We expose, for example, `launcher.DetermineRootDirectoryOverride` to correctly set the root directory flag. We could expose other portions of `launcher.ParseOptions` in new functions too.

Finally, we can consider category 1. Category 1 has the advantage of flexibility and logic consolidation -- just add the new flag into `launcher.ParseOptions` and `launcher.Options`. The drawback is maintainability. `launcher.Options` contains more and more options as time goes on, and it's not always clear which options are relevant to which subcommand versus to launcher proper.

## Decision

For flexibility and maintainability, I propose standardizing on category 2. Flags for running launcher will continue to live in `launcher.ParseOptions`/`launcher.Options`. Flags that are specific to only a subcommand will live in a flagset specific to that subcommand. Subcommand flags that exist in `launcher.Options` should be parsed via `launcher.ParseOptions`.

This decision leaves us with two subcommands that are not in line with this new standard -- `compactdb` and `uninstall`. `compactdb` will be updated to deprecate its `CompactDbMaxTx` flag from `launcher.Options` and move that flag into its own flagset. The `uninstall` command will be updated to use `launcher.ParseOptions`.

## Consequences

We will parse all our shared options in a standardized way, and have more specific flagsets for launcher subcommands to make it clear which flag is appropriate for which subcommand.

We do not anticipate any negative effects from deprecating the `CompactDbMaxTx` flag from `launcher.Options` and moving it into the `compactdb` subcommand only.

Flagsets for some subcommands, like `compactdb`, will have to become a little more verbose to list all of the flags from `launcher.ParseOptions` that they need.
