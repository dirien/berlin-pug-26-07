import * as k8s from "@pulumi/kubernetes";
import {PolicyPack, validateResourceOfType} from "@pulumi/policy";

new PolicyPack("kubernetes-typescript", {
    policies: [{
        name: "require-non-root-deployment",
        description: "Nginx deployment must not run as root.",
        enforcementLevel: "advisory",
        validateResource: validateResourceOfType(k8s.apps.v1.Deployment, (deploy, args, reportViolation) => {
            if (deploy.spec?.template.spec?.securityContext?.runAsNonRoot !== false) {
                reportViolation("Nginx deployment should not run as root.");
            }
        }),
    }],
});
