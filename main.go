package main

//go:generate ./resources/package.sh

import (
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/session"
	sparta "github.com/mweagle/Sparta"
	spartaCF "github.com/mweagle/Sparta/aws/cloudformation"
	spartaDecorators "github.com/mweagle/Sparta/decorator"
	"github.com/mweagle/SpartaGeekwire/service"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/sirupsen/logrus"
)

/*
================================================================================
╔╦╗╔═╗╔═╗╔═╗╦═╗╔═╗╔╦╗╔═╗╦═╗╔═╗
 ║║║╣ ║  ║ ║╠╦╝╠═╣ ║ ║ ║╠╦╝╚═╗
═╩╝╚═╝╚═╝╚═╝╩╚═╩ ╩ ╩ ╚═╝╩╚═╚═╝
================================================================================
*/
func serviceResourceDecorator(connections *service.Connections,
	websiteURL *gocf.StringExpr) sparta.ServiceDecoratorHookFunc {
	return func(context map[string]interface{},
		serviceName string,
		cfTemplate *gocf.Template,
		S3Bucket string,
		buildID string,
		awsSession *session.Session,
		noop bool,
		logger *logrus.Logger) error {

		// Add the dynamic S3 bucket, orphan it...
		s3Bucket := &gocf.S3Bucket{}
		s3Bucket.CorsConfiguration = &gocf.S3BucketCorsConfiguration{
			CorsRules: &gocf.S3BucketCorsRuleList{
				gocf.S3BucketCorsRule{
					AllowedOrigins: gocf.StringList(
						websiteURL,
						gocf.String("http://localhost:8002")),
					AllowedMethods: gocf.StringList(
						gocf.String(http.MethodGet),
						gocf.String(http.MethodPut),
					),
					MaxAge: gocf.Integer(3000),
					AllowedHeaders: gocf.StringList(
						gocf.String("Authorization"),
						gocf.String("Access-Control-Request-Method"),
						gocf.String("Access-Control-Request-Headers"),
						gocf.String("Content-Type"),
						gocf.String("Origin"),
					),
				},
			},
		}
		s3Resource := cfTemplate.AddResource(connections.S3UploadBucketResourceName,
			s3Bucket)
		s3Resource.DeletionPolicy = "Retain"
		// Add the CORS policy
		return nil
	}
}

func workflowHooks(connections *service.Connections,
	lambdaFunctions []*sparta.LambdaAWSInfo,
	websiteURL *gocf.StringExpr) *sparta.WorkflowHooks {
	// Setup the DashboardDecorator lambda hook
	workflowHooks := &sparta.WorkflowHooks{
		ServiceDecorators: []sparta.ServiceDecoratorHookHandler{
			spartaDecorators.DashboardDecorator(lambdaFunctions, 60),
			serviceResourceDecorator(connections, websiteURL),
		},
	}
	return workflowHooks
}

/*
================================================================================
╔═╗╔═╗╔═╗╦  ╦╔═╗╔═╗╔╦╗╦╔═╗╔╗╔
╠═╣╠═╝╠═╝║  ║║  ╠═╣ ║ ║║ ║║║║
╩ ╩╩  ╩  ╩═╝╩╚═╝╩ ╩ ╩ ╩╚═╝╝╚╝
================================================================================
*/

func main() {

	connections := &service.Connections{
		S3UploadBucketResourceName:     "S3UploadBucket",
		S3KeyspaceUploads:              "uploads",
		S3KeyspaceRekognitionArtifacts: "rekognition-artifacts",
		S3KeyspacePollyArtifacts:       "polly-artifacts",
		S3KeyspaceComprehendArtifacts:  "comprehend-artifacts",
		S3KeyspaceConsolidatedStatus:   "consolidated",
	}

	// Provision an S3 site
	s3Site, s3SiteErr := sparta.NewS3Site("./resources/dist")
	if s3SiteErr != nil {
		panic("Failed to create S3 Site")
	}
	// Register the function with the API Gateway
	apiStage := sparta.NewStage("v1")
	apiGateway := sparta.NewAPIGateway("SpartaGeekwire", apiStage)
	// Enable CORS s.t. the S3 site can access the resources
	apiGateway.CORSOptions = &sparta.CORSOptions{
		Headers: map[string]interface{}{
			"Access-Control-Allow-Headers": "Content-Type,X-Amz-Date,Authorization,X-Api-Key",
			"Access-Control-Allow-Methods": "*",
			"Access-Control-Allow-Origin": gocf.GetAtt(s3Site.CloudFormationS3ResourceName(),
				"WebsiteURL"),
		},
	}

	// Define the stack
	lambdaFunctions := service.New(connections, apiGateway)
	stackName := spartaCF.UserScopedStackName("SpartaGeekwire")
	sparta.MainEx(stackName,
		fmt.Sprintf("GeekWire service combines S3 with multiple AWS Services"),
		lambdaFunctions,
		apiGateway,
		s3Site,
		workflowHooks(connections,
			lambdaFunctions,
			gocf.GetAtt(s3Site.CloudFormationS3ResourceName(), "WebsiteURL")),
		false)
}
