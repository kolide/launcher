Launcher 0.5.0
---
* Add a local debug server. (#187)
* Fixed an issue(#101) where status logs would be cast as shapshot. (#192)
* Limit logs to a max message size of 2MB. (#191, #266)
* Set the timeout value for the osquery extension socket to 10s. (#194)
* Add ability to toggle logs between info/debug by sending SIGUSR2. (#195)
* Add ability to omit enrollment secret from launcher packages. (#201)
* New tables: kolide_munki_info, kolide_mdm_info, kolide_macho_info. (#217, #218, #244)
* Add cert pining support to launcher and package builder. (#254)
* Allow specifying custom certificate roots to launcher and package builder. (#263)
