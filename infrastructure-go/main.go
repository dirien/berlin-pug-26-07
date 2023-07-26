package main

import (
	"fmt"
	"os"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-eks/sdk/go/eks"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var (
	publicSubnetCidrs = []string{
		"172.31.0.0/20",
		"172.31.48.0/20",
	}
	availabilityZones = []string{
		"eu-central-1a",
		"eu-central-1b",
	}
)

const (
	clusterName       = "berlin-pug-eks-cluster"
	clusterTag        = "kubernetes.io/cluster/" + clusterName
	albNamespace      = "aws-lb-controller"
	albServiceAccount = "system:serviceaccount:" + albNamespace + ":aws-lb-controller-serviceaccount"
	ebsServiceAccount = "system:serviceaccount:kube-system:ebs-csi-controller-sa"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Create an AWS resource (S3 Bucket)
		vpc, err := ec2.NewVpc(ctx, "berlin-pug-vpc", &ec2.VpcArgs{
			CidrBlock: pulumi.String("172.31.0.0/16"),
		})
		if err != nil {
			return err
		}
		igw, err := ec2.NewInternetGateway(ctx, "berlin-pug-igw", &ec2.InternetGatewayArgs{
			VpcId: vpc.ID(),
		})
		if err != nil {
			return err
		}
		rt, err := ec2.NewRouteTable(ctx, "berlin-pug-rt", &ec2.RouteTableArgs{
			VpcId: vpc.ID(),
			Routes: ec2.RouteTableRouteArray{
				&ec2.RouteTableRouteArgs{
					CidrBlock: pulumi.String("0.0.0.0/0"),
					GatewayId: igw.ID(),
				},
			},
		})
		if err != nil {
			return err
		}

		var publicSubnetIDs pulumi.StringArray

		// Create a subnet for each availability zone
		for i, az := range availabilityZones {
			publicSubnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("berlin-pug-subnet-%d", i), &ec2.SubnetArgs{
				VpcId:                       vpc.ID(),
				CidrBlock:                   pulumi.String(publicSubnetCidrs[i]),
				MapPublicIpOnLaunch:         pulumi.Bool(true),
				AssignIpv6AddressOnCreation: pulumi.Bool(false),
				AvailabilityZone:            pulumi.String(az),
				Tags: pulumi.StringMap{
					"Name":                   pulumi.Sprintf("eks-public-subnet-%d", az),
					clusterTag:               pulumi.String("owned"),
					"kubernetes.io/role/elb": pulumi.String("1"),
				},
			})
			if err != nil {
				return err
			}
			_, err = ec2.NewRouteTableAssociation(ctx, fmt.Sprintf("berlin-pug-rt-association-%s", az), &ec2.RouteTableAssociationArgs{
				RouteTableId: rt.ID(),
				SubnetId:     publicSubnet.ID(),
			})
			if err != nil {
				return err
			}
			publicSubnetIDs = append(publicSubnetIDs, publicSubnet.ID())
		}

		cluster, err := eks.NewCluster(ctx, clusterName, &eks.ClusterArgs{
			Name:            pulumi.String(clusterName),
			VpcId:           vpc.ID(),
			SubnetIds:       publicSubnetIDs,
			InstanceType:    pulumi.String("t3.medium"),
			DesiredCapacity: pulumi.Int(2),
			MinSize:         pulumi.Int(1),
			MaxSize:         pulumi.Int(3),
			ProviderCredentialOpts: eks.KubeconfigOptionsArgs{
				ProfileName: pulumi.String("default"),
			},
			CreateOidcProvider: pulumi.Bool(true),
		})
		if err != nil {
			return err
		}

		ctx.Export("kubeconfig", pulumi.ToSecret(cluster.Kubeconfig))

		albRole, err := iam.NewRole(ctx, "berlin-pug-alb-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.All(cluster.Core.OidcProvider().Arn(), cluster.Core.OidcProvider().Url()).ApplyT(func(args []interface{}) (string, error) {
				arn := args[0].(string)
				url := args[1].(string)
				return fmt.Sprintf(`{
						"Version": "2012-10-17",
						"Statement": [
							{
								"Effect": "Allow",
								"Principal": {
									"Federated": "%s"
								},
								"Action": "sts:AssumeRoleWithWebIdentity",
								"Condition": {
									"StringEquals": {
										"%s:sub": "%s"
									}
								}
							}
						]
					}`, arn, url, albServiceAccount), nil
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}

		albPolicyFile, _ := os.ReadFile("./iam-policies/alb-iam-policy.json")
		albIAMPolicy, err := iam.NewPolicy(ctx, "berlin-pug-alb-policy", &iam.PolicyArgs{
			Policy: pulumi.String(albPolicyFile),
		}, pulumi.DependsOn([]pulumi.Resource{albRole}))
		if err != nil {
			return err
		}
		_, err = iam.NewRolePolicyAttachment(ctx, "berlin-pug-alb-role-attachment", &iam.RolePolicyAttachmentArgs{
			PolicyArn: albIAMPolicy.Arn,
			Role:      albRole.Name,
		}, pulumi.DependsOn([]pulumi.Resource{albIAMPolicy}))
		if err != nil {
			return err
		}

		provider, err := kubernetes.NewProvider(ctx, "berlin-pug-k8s-provider", &kubernetes.ProviderArgs{
			Kubeconfig:            cluster.KubeconfigJson,
			EnableServerSideApply: pulumi.Bool(true),
		}, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return err
		}

		ns, err := corev1.NewNamespace(ctx, albNamespace, &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(albNamespace),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/name": pulumi.String("aws-load-balancer-controller"),
				},
			},
		}, pulumi.Provider(provider), pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return err
		}
		sa, err := corev1.NewServiceAccount(ctx, "aws-lb-controller-sa", &corev1.ServiceAccountArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("aws-lb-controller-serviceaccount"),
				Namespace: ns.Metadata.Name(),
				Annotations: pulumi.StringMap{
					"eks.amazonaws.com/role-arn": albRole.Arn,
				},
			},
		}, pulumi.Provider(provider), pulumi.DependsOn([]pulumi.Resource{ns, cluster}))
		if err != nil {
			return err
		}

		_, err = helm.NewRelease(ctx, "aws-load-balancer-controller", &helm.ReleaseArgs{
			Chart:     pulumi.String("aws-load-balancer-controller"),
			Version:   pulumi.String("1.5.3"),
			Namespace: ns.Metadata.Name(),
			RepositoryOpts: helm.RepositoryOptsArgs{
				Repo: pulumi.String("https://aws.github.io/eks-charts"),
			},
			Values: pulumi.Map{
				"clusterName": cluster.EksCluster.ToClusterOutput().Name(),
				"region":      pulumi.String("eu-central-1"),
				"serviceAccount": pulumi.Map{
					"create": pulumi.Bool(false),
					"name":   sa.Metadata.Name(),
				},
				"vpcId": cluster.EksCluster.VpcConfig().VpcId(),
				"podLabels": pulumi.Map{
					"stack": pulumi.String("eks"),
					"app":   pulumi.String("aws-lb-controller"),
				},
			},
		}, pulumi.Provider(provider), pulumi.DependsOn([]pulumi.Resource{ns, sa, cluster}))
		if err != nil {
			return err
		}

		return nil
	})
}
