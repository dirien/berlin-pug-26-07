"use strict";
const aws = require("@pulumi/aws");
const policy = require("@pulumi/policy");

new policy.PolicyPack("aws-javascript", {
    policies: [{
        name: "vpc-check-cidr",
        description: "Checks that the VPC CIDR block is set to a valid value",
        enforcementLevel: "mandatory",
        validateResource: policy.validateResourceOfType(aws.ec2.Vpc, (vpcCheck, args, reportViolation) => {
                if (vpcCheck.cidrBlock !== "172.31.0.0/16") {
                    reportViolation("VPC CIDR block must be set to 172.31.0.0/16");
                }
            }
        ),
    }],
});
