<a name="unreleased"></a>
## [Unreleased]

<a name="v0.4.0"></a>
## [v0.4.0] - 2021-06-07
### Changed
- Upgrade workflows and deps (#80)
- Execute workflow only when addon checksum changes (#79)

<a name="v0.3.1"></a>
## [v0.3.1] - 2020-10-06
### Fixed
- Upgrade Argo to v2.11.1 (#72)
- Improve Watchers to only look for owned resources (#72)
### Changed
- Remove common app.kubernetes.io/version labels from Kustomize (#71)

<a name="v0.3.0"></a>
## [v0.3.0] - 2020-09-23
### Fixed
- Change Failed status to Pending for dependant addons (#63)
- Upgrade Argo and inject deadline for workflows. (#64) (#69)
### Changed
- Various improvements (#60) (#61) (#62)

<a name="v0.2.1"></a>
## [v0.2.1] - 2020-04-13
### Fixed
- Patch the master CI build (#48) (#56) (#58)
- Preserve labels in Workflow templates (#49)

<a name="v0.2.0"></a>
## [v0.2.0] - 2020-03-31
### Changed
- Updated to Go 1.14 
- Argo Workflow controller updated to 2.4.3
- Upgraded Kubebuilder to v2.3.0 
### Fixed
- Various fixes and improvements (#46)

<a name="v0.1.0"></a>
## [v0.1.0] - 2019-11-27
### Changed
- Argo Workflow controller updated to 2.4.2 
- Upgraded Kubebuilder to v2.2.0 
### Fixed
- Various fixes and improvements (#39)

<a name="v0.0.2"></a>
## [v0.0.2] - 2019-09-03
### Changed
- Moved project api version to keikoproj.io

<a name="v0.0.1"></a>
## [v0.0.1] - 2019-08-14
### Added
- Initial Release of Addon Manager

[Unreleased]: https://github.com/keikoproj/addon-manager/compare/v0.4.0...HEAD
[v0.4.0]: https://github.com/keikoproj/addon-manager/compare/v0.3.1...v0.4.0
[v0.3.1]: https://github.com/keikoproj/addon-manager/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/keikoproj/addon-manager/compare/v0.3.0...v0.2.1
[v0.2.1]: https://github.com/keikoproj/addon-manager/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/keikoproj/addon-manager/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/keikoproj/addon-manager/compare/v0.1.0...v0.0.2
[v0.0.2]: https://github.com/keikoproj/addon-manager/compare/v0.0.1...v0.0.2
[v0.0.1]: https://github.com/keikoproj/addon-manager/releases/tag/v0.0.1
