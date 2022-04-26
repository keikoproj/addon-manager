<a name="unreleased"></a>
## [Unreleased]

<a name="v0.5.0"></a>
## [v0.5.0] - 2022-04-26
### Fixed
- Build Fix (#102)
- Fix UTC build date (#135)
### Changed
- Upgrade to Go 1.17+ (#116)
- Upgrade Kubernetes APIs (apimachinery, api, etc.) to 0.21 (#116)
- Upgrade Argo to v3.2.6 (#116)
- move api/v1alpha1 to api/addon/v1alpha1; add "comment marker" (#116)
- use code-generator generate API(s) clientset/informer/listers and put into pkg/client directory. (#116)
- add code-generator into Makefile generate (#116)
- update build badge to use GitHub Action status (#109)
- Migrate to GitHub Actions to gate PRs. (#105)
- bumps actions/checkout from 2 to 3 (#119)
- addon-manager refactor pkg/ directory (#123)
- Bump actions/setup-go from 2 to 3 (#132)
- Bump codecov/codecov-action from 2.1.0 to 3 (#128)

<a name="v0.4.3"></a>
## [v0.4.3] - 2021-07-15
### Fixed
- Fix delete flow and add tests (#90)
### Changed
- Record Addon dependency validation errors as state (#88)
- Requeue addon for deps not installed error (#91)

<a name="v0.4.2"></a>
## [v0.4.2] - 2021-06-09
### Changed
- Increase Addon TTL to 1-hour and reset status per checksum change (#86)

<a name="v0.4.1"></a>
## [v0.4.1] - 2021-06-08
### Changed
- Add a conflict retry on status updates (#82)
- Always reset the timer whenever checksum changes (#83)

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

[Unreleased]: https://github.com/keikoproj/addon-manager/compare/v0.4.2...HEAD
[v0.4.2]: https://github.com/keikoproj/addon-manager/compare/v0.4.1...v0.4.2
[v0.4.1]: https://github.com/keikoproj/addon-manager/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/keikoproj/addon-manager/compare/v0.3.1...v0.4.0
[v0.3.1]: https://github.com/keikoproj/addon-manager/compare/v0.3.0...v0.3.1
[v0.3.0]: https://github.com/keikoproj/addon-manager/compare/v0.3.0...v0.2.1
[v0.2.1]: https://github.com/keikoproj/addon-manager/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/keikoproj/addon-manager/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/keikoproj/addon-manager/compare/v0.1.0...v0.0.2
[v0.0.2]: https://github.com/keikoproj/addon-manager/compare/v0.0.1...v0.0.2
[v0.0.1]: https://github.com/keikoproj/addon-manager/releases/tag/v0.0.1
