# Addon Manager
[![Maintenance](https://img.shields.io/badge/Maintained%3F-yes-green.svg)][GithubMaintainedUrl]
[![PR](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)][GithubPrsUrl]
[![slack](https://img.shields.io/badge/slack-join%20the%20conversation-ff69b4.svg)][SlackUrl]

![version](https://img.shields.io/badge/version-0.2.0-blue.svg?cacheSeconds=2592000)
[![Build Status][BuildStatusImg]][BuildMasterUrl]
[![codecov][CodecovImg]][CodecovUrl]
[![Go Report Card][GoReportImg]][GoReportUrl]

Addons are critical components within a Kubernetes cluster that provide critical services needed by applications like 
DNS, Ingress, Metrics, Logging, etc. Addon Manager provides a CRD for lifecycle management of such addons using 
Argo Workflows.

## Dependencies
* kubernetes-1.11.1+
* kubectl-1.14+

## Installation
To use: `kubectl kustomize github.com/keikoproj/addon-manager.git/config/default | kubectl apply -f -`

## Usage example
An Addon describes a kubernetes resource-based application that is deployed to a cluster. The Addon CRD defines a spec 
with some optional and required fields, and a lifecycle where most of the addon may be contained. Internally, 
Argo Workflows is used for each lifecycle step to specify and submit the resources. 
The design spec for the Addon CRD is as follows:

```yaml
apiVersion: addonmgr.keikoproj.io/v1alpha1
kind: Addon
metadata:
  name: fluentd-addon
  namespace: addon-manager-system
spec:
  pkgName: core/fluentd
  pkgVersion: v0.0.1
  pkgType: composite
  pkgDescription: Company fluentd addon.
  pkgDeps: 
    argoproj/workflows: v2.2.1
  params:
    namespace: mynamespace
    clusterContext:
      clusterName: "my-test-cluster"
      clusterRegion: "us-west-2"
    data:
      hec_splunk_server: hec.splunk.example.com
  selector:
    matchLabels:
      app.kubernetes.io/name: fluentd
      app.kubernetes.io/version: "1.0.0"
  lifecycle:
    prereqs:
      template: |
        apiVersion: argoproj.io/v1alpha1
        kind: Workflow
        ...
    install:
      template: |
        apiVersion: argoproj.io/v1alpha1
        kind: Workflow
        ...
    delete:
      template: | 
        apiVersion: argoproj.io/v1alpha1
        kind: Workflow
        ...
    validate:
      template: |
        apiVersion: argoproj.io/v1alpha1
        kind: Workflow
        ...
```
To submit: `kubectl apply -f addon.yaml`

The 4 stages of the addon lifecycle (prereqs, install, validate, delete) have 3 fields, Name, Role, and Template. 
The template is specified as an inline <a href="https://github.com/argoproj/argo/" target="_blank">Argo Workflow ![alt [^]][ext_link]</a>. The workflow should specify the Kubernetes resources being 
submitted as part of each step, and include the appropriate kubernetes client commands that submit those resources to 
the server. Here's an example of such

```yaml
...
    prereqs:
      template: |
        apiVersion: argoproj.io/v1alpha1
        kind: Workflow
        metadata:
          generateName: prereqs-minion-manager-
        spec:
          entrypoint: entry
          serviceAccountName: addon-manager-workflow-installer-sa
          templates:
          - name: entry
            steps:
            - - name: prereq-resources
                template: submit
                arguments:
                  artifacts:
                  - name: doc
                    path: /tmp/doc
                    raw:
                      data: |
                        apiVersion: v1
                        kind: Namespace
                        metadata:
                          name: "{{workflow.parameters.namespace}}"
                        ---
                        apiVersion: v1
                        kind: ServiceAccount
                        metadata:
                          name: example-sa
                          namespace: "{{workflow.parameters.namespace}}"
          - name: submit
            inputs:
              artifacts:
                - name: doc
                  path: /tmp/doc
            container:
              image: expert360/kubectl-awscli:v1.11.2
              command: [sh, -c]
              args: ["kubectl apply -f /tmp/doc"]
```

The Addon Manager provides parameter injection to all lifecycle workflows to get rid of the need to input the parameters 
into each workflow. Before the workflows are run, all key-value pairs in the addon spec.params are made into global 
workflow parameters. This means their values are accessible like so: "{{workflow.parameters.NAME}}". It's important to 
make sure when referencing parameters that the templated value is escaped, either with quotes or if it's included in a 
string literal; if this is not done, you may experience a failed workflow due to the worflow failing to be parsed 
correctly.

Generally, there are a set of best practices defined that make defining an Addon CR straightforward:
* Each addon (with a few exceptions) should be deployed to its own namespace. This is done by specifying a namespace name 
in `spec.params.namespace`, and then templating that into each lifecycle workflow where there are namespaced resources, 
like in the example above.
* Best-practice for the prereqs lifecycle step workflow: if the addon has any namespaced resources, the namespace in 
question should be specified first in the prereqs workflow. Following the namespace, all other non-deployable resources 
should be specified by priotity in the prereqs workflow as well.
* Best-practice for the install lifecycle step workflow: all deployable resources (deployments, services, statefulsets, 
replicasets, daemonsets, etc.) should be supplied as part of this workflow.

### Get Addons
```bash
kubectl get addons -n addon-manager-system

NAME                       PACKAGE                    VERSION       STATUS           AGE
addon-manager-argo-addon   addon-argo-workflow        v2.2.1        Succeeded        14m
cluster-autoscaler         cluster-autoscaler-addon   v0.1          Pending          1m
event-router               event-router               v0.2          Pending          1m
external-dns               external-dns               v0.2          Pending          1m
fluentd                    core/fluentd-addon         v0.0.1        Pending          1m
...
```

### Delete Addon
To delete: `kubectl delete -f addon.yaml`

## Addonctl
The Addon Manager is distributed with the addonctl binary which allows a default Addon CR generation given spec 
parameters yaml resource files, and python scripts. Pre-alpha currently, this tool can be more useful for initial addon 
generation, allowing the user to then manually modify it. 

To use: `addonctl --help`
```bash
A control plane for managing addons

Usage:
  addonctl [command]

Available Commands:
  create      Create the addon resource with the supplied arguments
  help        Help about any command

Flags:
  -c, --channel string          Channel for the addon package
      --cluster-name string     Name of the cluster context being used
      --cluster-region string   Cluster region
      --deps string             Comma seperated dependencies list in the format 'pkgName:pkgVersion'
      --desc string             Description of the addon
      --dryrun                  Outputs the addon spec but doesn't submit
  -h, --help                    help for addonctl
      --install string          File or directory of resource yaml to submit as install step
  -n, --namespace string        Namespace where the addon will be deployed
  -p, --params string           Params to supply to the resource yaml
      --prereqs string          File or directory of resource yaml to submit as prereqs step
      --secrets string          Comma seperated list of secret names which are validated as part ofthe addon-manager-system namespace
      --selector string         Selector applied to all resources?
  -t, --type string             Addon package type
  -v, --version string          Addon package version

Use "addonctl [command] --help" for more information about a command.
```

### Addonctl Example
```bash
addonctl create my-addon -n my-addon-ns \
  --type composite \
  --version v0.2 \
  --cluster-name my.cluster.k8s.local \
  --cluster-region us-west-2 \
  --selector app:myaddon \
  --prereqs ./prereq_resources.yaml \
  --install ./install_resources.yaml \
  --dryrun
```

## Release History
* 0.2.0
  * Update api version to Keikoproj
* 0.1.0
  * Release alpha version of addon-manager

## ❤ Contributing ❤

Please see [CONTRIBUTING.md](.github/CONTRIBUTING.md).

## Developer Guide

Please see [DEVELOPER.md](.github/DEVELOPER.md).

## Other KeikoProj Projects
[Instance Manager][InstanceManagerUrl] -
[Kube Forensics][KubeForensicsUrl] -
[Active Monitor][ActiveMonitorUrl] -
[Upgrade Manager][UpgradeManagerUrl] -
[Minion Manager][MinionManagerUrl] -
[Governor][GovernorUrl]

<!-- Markdown link -->
[install]: docs/README.md
[ext_link]: https://upload.wikimedia.org/wikipedia/commons/d/d9/VisualEditor_-_Icon_-_External-link.svg

[InstanceManagerUrl]: https://github.com/keikoproj/instance-manager
[KubeForensicsUrl]: https://github.com/keikoproj/kube-forensics
[ActiveMonitorUrl]: https://github.com/keikoproj/active-monitor
[UpgradeManagerUrl]: https://github.com/keikoproj/upgrade-manager
[MinionManagerUrl]: https://github.com/keikoproj/minion-manager
[GovernorUrl]: https://github.com/keikoproj/governor

[GithubMaintainedUrl]: https://github.com/keikoproj/addon-manager/graphs/commit-activity
[GithubPrsUrl]: https://github.com/keikoproj/addon-manager/pulls
[SlackUrl]: https://keikoproj.slack.com/messages/addon-manager

[BuildStatusImg]: https://travis-ci.org/keikoproj/addon-manager.svg?branch=master
[BuildMasterUrl]: https://travis-ci.org/keikoproj/addon-manager

[CodecovImg]: https://codecov.io/gh/keikoproj/addon-manager/branch/master/graph/badge.svg
[CodecovUrl]: https://codecov.io/gh/keikoproj/addon-manager

[GoReportImg]: https://goreportcard.com/badge/github.com/keikoproj/addon-manager
[GoReportUrl]: https://goreportcard.com/report/github.com/keikoproj/addon-manager
