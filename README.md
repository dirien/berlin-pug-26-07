# Berlin PUG Demo

This repository contains the demo code for the Berlin PUG.
See https://www.meetup.com/meetup-group-ziwnrlfj/events/293323160/ for more details.

## Infrastructure Deployment

The infrastructure deployment will be done using Pulumi AWS and EKS provider. As language we will use Go.

```bash
cd infrastructure-go

pulumi preview [--policy-pack ../policy-as-code/infra]
pulumi up [--policy-pack ../policy-as-code/infra]
```

After the infrastructure is deployed, we need to get the kubeconfig from the stack output and save it to a file.

```bash
pulumi stack output kubeconfig --show-secrets > kubeconfig.yaml
```

This is not a necessary step as we're going to use Pulumi Stack References to get the kubeconfig from the infrastructure
stack. See https://www.pulumi.com/learn/building-with-pulumi/stack-references/ for more details.

## Application Deployment

This deployment will be done using Pulumi AI. Head over to https://pulumi.com/ai and enter following prompt:

```text
Imagine you are a Kubernetes application developer and need to use a stack reference (dirien/berlin-pug-infrastructure-go/dev) to
get Kubeconfig from a different stack.
Then you create a Nginx (use the latest tag for the image) deployment with a custom config map, which contains "Hello
Berlin PUG" as part of the nginx.conf location tag inside the server tag under http. This config map should then be mounted on /etc/nginx/. Finally, expose this deployment via a service
of type nodeport. Create an ingress also. The ingress class should be "alb". Add the missing annotations for the
application load Balancer "internet-facing" and "instance".

Create only one Kubernetes provider and pass it as dependency to the resources.

Finally Export the address of the ingress as output by interploating the `http` protocol to it.
```

Here is a link to a working solution:
https://www.pulumi.com/ai/?convid=89a5a223-34bd-4b16-acb2-513f3cb2257c

```bash
cd application-ts
pulumi preview [--policy-pack ../policy-as-code/app]
```
