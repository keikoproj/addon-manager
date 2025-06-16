<a name="unreleased"></a>
## [Unreleased]


<a name="v0.9.0"></a>
## [v0.9.0] - 2024-07-15

## What's Changed
* Update codecov GHA config by @tekenstam in https://github.com/keikoproj/addon-manager/pull/333
* Bump golang.org/x/net from 0.24.0 to 0.26.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/327
* Bump k8s.io/api from 0.29.0 to 0.29.6 by @dependabot in https://github.com/keikoproj/addon-manager/pull/330
* chore: update dev docs to mention proper make kind-cluster by @kevdowney in https://github.com/keikoproj/addon-manager/pull/320
* Bump github.com/argoproj/argo-workflows/v3 from 3.4.11 to 3.5.10 by @dependabot in https://github.com/keikoproj/addon-manager/pull/341
* chore: standardize GitHub templates by @tekenstam in https://github.com/keikoproj/addon-manager/pull/362
* Upgrade to Go 1.24 by @tekenstam in https://github.com/keikoproj/addon-manager/pull/354
* Bump golang.org/x/net from 0.26.0 to 0.36.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/352
* Bump github.com/argoproj/argo-workflows/v3 from 3.5.10 to 3.5.13 by @dependabot in https://github.com/keikoproj/addon-manager/pull/355
* Bump codecov/codecov-action from 4 to 5 by @dependabot in https://github.com/keikoproj/addon-manager/pull/353
* Bump github.com/onsi/gomega from 1.33.0 to 1.36.3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/359
* Bump k8s.io/apimachinery from 0.29.6 to 0.29.15 by @dependabot in https://github.com/keikoproj/addon-manager/pull/361
* Update Addon Manager for Kubernetes 1.32 Compatibility by @tekenstam in https://github.com/keikoproj/addon-manager/pull/364
* fix: change app label for workflow-controller by @kevdowney in https://github.com/keikoproj/addon-manager/pull/342
* Improve Code Coverage Configuration and Testing by @tekenstam in https://github.com/keikoproj/addon-manager/pull/365
* Bump github.com/argoproj/argo-workflows/v3 from 3.5.10 to 3.6.5 by @dependabot in https://github.com/keikoproj/addon-manager/pull/356
* fix: Remove kiam annotations by @kevdowney in https://github.com/keikoproj/addon-manager/pull/366
* Bump github.com/spf13/cobra from 1.8.1 to 1.9.1 by @dependabot in https://github.com/keikoproj/addon-manager/pull/369
* Bump golang.org/x/net from 0.37.0 to 0.38.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/371
* Bump k8s.io/client-go from 0.32.1 to 0.32.3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/367
* Bump k8s.io/apiextensions-apiserver from 0.32.1 to 0.32.3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/370
* chore(deps): bump github.com/Masterminds/semver/v3 from 3.2.1 to 3.3.1 by @dependabot in https://github.com/keikoproj/addon-manager/pull/372
* chore(deps): bump github.com/onsi/ginkgo/v2 from 2.23.3 to 2.23.4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/373
* chore(deps): bump github.com/onsi/gomega from 1.36.3 to 1.37.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/374
* chore(deps): bump golang.org/x/net from 0.38.0 to 0.39.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/375
* chore(deps): bump k8s.io/client-go from 0.32.3 to 0.32.4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/380
* chore(deps): bump github.com/argoproj/argo-workflows/v3 from 3.6.5 to 3.6.10 by @dependabot in https://github.com/keikoproj/addon-manager/pull/388

**Full Changelog**: https://github.com/keikoproj/addon-manager/compare/v0.8.2...v0.9.0

<a name="v0.8.2"></a>
## [v0.8.2] - 2024-06-11
## What's Changed
* Bump google.golang.org/grpc from 1.56.2 to 1.56.3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/296
* Bump actions/setup-go from 4 to 5 by @dependabot in https://github.com/keikoproj/addon-manager/pull/305
* Bump github/codeql-action from 2 to 3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/306
* Bump codecov/codecov-action from 3 to 4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/308
* Bump k8s.io/apiextensions-apiserver from 0.25.14 to 0.25.16 by @dependabot in https://github.com/keikoproj/addon-manager/pull/300
* Bump github.com/spf13/cobra from 1.7.0 to 1.8.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/310
* Bump github.com/go-logr/logr from 1.2.4 to 1.4.1 by @dependabot in https://github.com/keikoproj/addon-manager/pull/311
* chore: upgraded to controller-runtime 0.17.2 and ginkgo v2 by @ccpeng in https://github.com/keikoproj/addon-manager/pull/319
* Bump goreleaser/goreleaser-action from 5 to 6 by @dependabot in https://github.com/keikoproj/addon-manager/pull/324
* chore: update dev tools by @kevdowney in https://github.com/keikoproj/addon-manager/pull/326

**Full Changelog**: https://github.com/keikoproj/addon-manager/compare/v0.8.1...v0.8.2

<a name="v0.8.1"></a>
## [v0.8.1] - 2023-10-30
### Fixed
- Remove old workflows on addon checksum change (#297)

### Changed
- Bump golang.org/x/net from 0.15.0 to 0.17.0 (#289)
- Bump github.com/onsi/gomega from 1.27.10 to 1.28.0 (#286)

<a name="v0.8.0"></a>
## [v0.8.0] - 2023-09-19

## What's Changed
* Unit tests for helpers.go by @estela-ramirez in https://github.com/keikoproj/addon-manager/pull/225
* Bump actions/setup-go from 3 to 4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/227
* Bump golang.org/x/text from 0.3.6 to 0.3.8 by @dependabot in https://github.com/keikoproj/addon-manager/pull/216
* go mod tidy -compat=1.17 by @kevdowney in https://github.com/keikoproj/addon-manager/pull/237
* Bump github.com/Masterminds/semver/v3 from 3.2.0 to 3.2.1 by @dependabot in https://github.com/keikoproj/addon-manager/pull/236
* update GH actions/checkout to use v3 by @kevdowney in https://github.com/keikoproj/addon-manager/pull/238
* Cleanup go.mod by @kevdowney in https://github.com/keikoproj/addon-manager/pull/239
* Bump github.com/spf13/cobra from 1.6.1 to 1.7.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/241
* Bump golang.org/x/net from 0.0.0-20211209124913-491a49abca63 to 0.10.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/240
* updating argo workflows controller to v3.4.8 and Go to 1.19 by @ccpeng in https://github.com/keikoproj/addon-manager/pull/244
* Local build fixes by @kevdowney in https://github.com/keikoproj/addon-manager/pull/250
* chore: refactoring so that addon controller flow won't inquire from workflow obj by @ccpeng in https://github.com/keikoproj/addon-manager/pull/251
* fix: update cronjob to non-deprecated version by @backjo in https://github.com/keikoproj/addon-manager/pull/256
* Bump k8s.io/apiextensions-apiserver from 0.23.5 to 0.23.17 by @dependabot in https://github.com/keikoproj/addon-manager/pull/245
* Bump github.com/go-logr/logr from 1.2.3 to 1.2.4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/247
* Bump k8s.io/api from 0.24.3 to 0.24.17 by @dependabot in https://github.com/keikoproj/addon-manager/pull/260
* Bump k8s.io/client-go from 0.24.3 to 0.24.17 by @dependabot in https://github.com/keikoproj/addon-manager/pull/261
* Bump docker/login-action from 2 to 3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/268
* Bump goreleaser/goreleaser-action from 4 to 5 by @dependabot in https://github.com/keikoproj/addon-manager/pull/266
* Bump docker/setup-buildx-action from 2 to 3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/267
* Bump codecov/codecov-action from 3 to 4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/265
* chore: ignore sigs.k8s.io minor version by @kevdowney in https://github.com/keikoproj/addon-manager/pull/269
* Bump docker/setup-qemu-action from 2 to 3 by @dependabot in https://github.com/keikoproj/addon-manager/pull/264
* Bump actions/checkout from 3 to 4 by @dependabot in https://github.com/keikoproj/addon-manager/pull/270
* Downgrade to codecov-action@v3 by @tekenstam in https://github.com/keikoproj/addon-manager/pull/274
* Bump github.com/argoproj/argo-workflows/v3 from 3.4.8 to 3.4.11 by @dependabot in https://github.com/keikoproj/addon-manager/pull/271
* Bump github.com/onsi/gomega from 1.19.0 to 1.27.10 by @dependabot in https://github.com/keikoproj/addon-manager/pull/273
* Bump golang.org/x/net from 0.10.0 to 0.15.0 by @dependabot in https://github.com/keikoproj/addon-manager/pull/272
* Update client-go to v0.25.12 by @tekenstam in https://github.com/keikoproj/addon-manager/pull/275
* Update Kubernetes runtime and build tools by @tekenstam in https://github.com/keikoproj/addon-manager/pull/277
* Bump k8s.io/apimachinery from 0.25.12 to 0.25.14 by @dependabot in https://github.com/keikoproj/addon-manager/pull/278
* Bump k8s.io/api from 0.25.12 to 0.25.14 by @dependabot in https://github.com/keikoproj/addon-manager/pull/281

## New Contributors
* @estela-ramirez made their first contribution in https://github.com/keikoproj/addon-manager/pull/225
* @backjo made their first contribution in https://github.com/keikoproj/addon-manager/pull/256
* @tekenstam made their first contribution in https://github.com/keikoproj/addon-manager/pull/274

**Full Changelog**: https://github.com/keikoproj/addon-manager/compare/v0.7.2...v0.8.0-rc.3

<a name="v0.7.2"></a>
## [v0.7.2] - 2023-03-10
### Fixed
- Recover from panics in addon workflow controller (#222)

### Changed
- Various improvements (#190) (#191) (#193) (#206) (#208) 
- Updated Dependencies (#195) (#197) (#198) (#203) (#214)

### Added
- Add goreleaser as prereqs developer doc (#188)
- Add codeql.yml workflow (#201)

### Removed
- Delete old CHANGELOG (#192)
- Remove KOPS test cluster scripts (#215)

<a name="v0.7.1"></a>
## [v0.7.1] - 2023-01-15
### Fixed
- Fix delete flows (#179)

### Changed
- Various improvements (#178) (#181) (#182) (#183) (#184) (#185)

<a name="v0.7.0"></a>
## [v0.7.0] - 2022-12-19
### Fixed
- Fix local bdd testing (#170) 
- Refactor addon status updates (#173) 

### Changed
- Bump actions/checkout from 2 to 3.1.0 (#160)
- Change to use controllerutil finalizer methods (#165) 
- Use controller-gen version v0.4.1 (#166)
- Switch to emissary workflow-executor and workflow-controller v3.3.x (#168)
- Bump actions/checkout from 3.1.0 to 3.2.0 (#172)
- Bump goreleaser/goreleaser-action from 3 to 4 (#171)

### Added
- Add addon-manager architecture diagram (#169)

<a name="v0.6.2"></a>
## [v0.6.2] - 2022-08-01
### Fixed
-  Remove verbose logging for workflow reconcile (#156)

<a name="v0.6.1"></a>
## [v0.6.1] - 2022-08-01
### Fixed
-  Handle notfound workflow use case (#154)

<a name="v0.6.0"></a>
## [v0.6.0] - 2022-07-29
### Fixed
-  Limit workflow watches to namespace scope (#150)

<a name="v0.5.2"></a>
## [v0.5.2] - 2022-06-07
### Fixed
- Fix workflow controller watcher and rbac (#147)

<a name="v0.5.1"></a>
## [v0.5.1] - 2022-05-31
### Fixed
- Bump github.com/argoproj/argo-workflows/v3 from 3.2.6 to 3.2.11 (#142)
- Upgrade Argo Workflows to v3.2.11 (#143)
- Fix GHSA-hp87-p4gw-j4gq security issue 7 (#144)
### Changed
- Update addon status based on workflow lifecycle and status change (#126)
- Bump docker/setup-buildx-action from 1 to 2 (#138)
- Bump docker/login-action from 1 to 2 (#139)
- Bump docker/setup-qemu-action from 1 to 2 (#140)
- Bump goreleaser/goreleaser-action from 2 to 3 (#141)

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

[Unreleased]: https://github.com/keikoproj/addon-manager/compare/v0.8.1...HEAD
[v0.8.1]: https://github.com/keikoproj/addon-manager/compare/v0.8.0...v0.8.1
[v0.8.0]: https://github.com/keikoproj/addon-manager/compare/v0.7.2...v0.8.0
[v0.7.2]: https://github.com/keikoproj/addon-manager/compare/v0.7.1...v0.7.2
[v0.7.1]: https://github.com/keikoproj/addon-manager/compare/v0.7.0...v0.7.1
[v0.7.0]: https://github.com/keikoproj/addon-manager/compare/v0.6.2...v0.7.0
[v0.6.2]: https://github.com/keikoproj/addon-manager/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/keikoproj/addon-manager/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/keikoproj/addon-manager/compare/v0.5.2...v0.6.0
[v0.5.2]: https://github.com/keikoproj/addon-manager/compare/v0.5.1...v0.5.2
[v0.5.1]: https://github.com/keikoproj/addon-manager/compare/v0.5.0...v0.5.1
[v0.5.0]: https://github.com/keikoproj/addon-manager/compare/v0.4.3...v0.5.0
[v0.4.3]: https://github.com/keikoproj/addon-manager/compare/v0.4.2...v0.4.3
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
