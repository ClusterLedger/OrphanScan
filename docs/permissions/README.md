# Read-only permissions

[Back to README](../../README.md) · [Usage](../usage.md) · [Credential handling](../../SECURITY.md#credential-handling)

## AWS

Give the scan identity the actions in [aws-iam-policy.json](aws-iam-policy.json). The discovery APIs need account-level listing; the example uses `Resource: "*"`. It contains seven AWS Describe actions and no write or deletion action.

Use the profile connected to that identity:

```bash
orphanscan --sources=aws --aws-profile audit --regions us-east-1
```

The standard AWS credential chain handles environment credentials, profiles, SSO, and role assumption. Authentication helpers and source identities can require separate permissions, such as permission to assume the audit role. The scanner does not request permission to inspect billing or change infrastructure.

The policy includes `ec2:DescribeRegions` for `--all-regions` and for automatic discovery when no AWS region is configured. Explicit `--regions` lists and a configured profile/environment region do not require that action. Discovery includes enabled regions in the current partition and never changes their opt-in status.

```bash
orphanscan --sources=aws --aws-profile audit --all-regions
```

The configured region supplies the discovery endpoint. With no configured region, discovery starts in commercial AWS at `us-east-1`. For another partition, configure a region from that partition and pass `--all-regions`. A discovery failure is reported as failed AWS configuration, with no automatic fallback to narrower coverage.

Official references: [EC2](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ec2.html), [Classic ELB](https://docs.aws.amazon.com/service-authorization/latest/reference/list_elb.html), and [ELBv2](https://docs.aws.amazon.com/service-authorization/latest/reference/list_elbv2.html).

## Kubernetes

The selected context needs cluster-wide `list` on `pods` and `persistentvolumeclaims`. The scanner reads every Pod phase and paginates both lists.

An administrator can adapt [kubernetes.yaml](kubernetes.yaml), replacing the example group with an existing authenticated audit group. Applying that example creates the role and binding; it is a separate administrator setup action. OrphanScan itself does not create RBAC objects or accounts.

Verify effective access for the selected context before scanning:

```bash
kubectl --context audit auth can-i list pods --all-namespaces
kubectl --context audit auth can-i list persistentvolumeclaims --all-namespaces
orphanscan --sources=kubernetes --context audit
```

See [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/). Namespace-only permissions will produce an incomplete or failed scan because OrphanScan lists across the cluster.
