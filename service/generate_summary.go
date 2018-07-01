package service

import (
	"context"
	"encoding/json"
	"fmt"

	awsLamdaEvents "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/service/rekognition"
	sparta "github.com/mweagle/Sparta"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/sirupsen/logrus"
)

type summaryInfo struct {
	Rekognition *rekognition.DetectLabelsOutput `json:"rekognition"`
	Polly       []byte                          `json:"polly"`
}

/*
================================================================================
╦  ╔═╗╔╦╗╔╗ ╔╦╗╔═╗
║  ╠═╣║║║╠╩╗ ║║╠═╣
╩═╝╩ ╩╩ ╩╚═╝═╩╝╩ ╩
================================================================================
Listen for PutObject events and submit the image to Rekognition
*/
func (gws *ServicefulService) onS3PutGenerateSummary(ctx context.Context, s3Event awsLamdaEvents.S3Event) error {
	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)

	// Process all the events...
	handler := func(ctx context.Context, event awsLamdaEvents.S3EventRecord) (interface{}, error) {
		// Super - what's the base name?
		baseName := gws.baseKeyname(event.S3.Object.Key)

		// Get the rekognition data
		keyPath := fmt.Sprintf("%s/%s",
			gws.connections.S3KeyspaceRekognitionArtifacts,
			baseName)

		rekognitionData, rekognitionDataErr := gws.getS3Object(ctx,
			event.S3.Bucket.Name,
			keyPath)
		if rekognitionDataErr != nil {
			return nil, rekognitionDataErr
		}

		rekognitionResponse := rekognition.DetectLabelsOutput{}
		unmarshalErr := json.Unmarshal(rekognitionData, &rekognitionResponse)
		if unmarshalErr != nil {
			return nil, unmarshalErr
		}
		// Then get the Polly data....
		keyPath = fmt.Sprintf("%s/%s",
			gws.connections.S3KeyspacePollyArtifacts,
			baseName)
		pollyData, pollyDataErr := gws.getS3Object(ctx,
			event.S3.Bucket.Name,
			keyPath)
		if pollyDataErr != nil {
			return nil, pollyDataErr
		}
		// And Base64Encode the Polly Data, which is implicit since it's a
		// []byte in the struct
		summary := summaryInfo{
			Rekognition: &rekognitionResponse,
			Polly:       pollyData,
		}
		outputKey := fmt.Sprintf("%s/%s",
			gws.connections.S3KeyspaceConsolidatedStatus,
			baseName)

		tags := map[string]string{
			tagNameAccess: tagAccessPublic,
		}
		putErr := gws.putJSONObjectToS3(ctx,
			event.S3.Bucket.Name,
			outputKey,
			&summary,
			tags)
		return nil, putErr
	}
	handleResult, handleErr := gws.handleS3Records(ctx, s3Event, handler)
	logger.WithField("Results", handleResult).Info("S3 event results")
	return handleErr
}

////////////////////////////////////////////////////////////////////////////////
// Policy so that the files we tag are properly marked public
func s3BucketTagPublicAccessDecorator(connections *Connections) sparta.TemplateDecoratorHookFunc {
	return func(serviceName string,
		lambdaResourceName string,
		lambdaResource gocf.LambdaFunction,
		resourceMetadata map[string]interface{},
		S3Bucket string,
		S3Key string,
		buildID string,
		cfTemplate *gocf.Template,
		context map[string]interface{},
		logger *logrus.Logger) error {

		//////////////////////////////////////////////////////////////////////////////
		// 2 - Add a bucket policy to enable anonymous access, as the PublicRead
		// canned ACL doesn't seem to do what is implied.
		// TODO - determine if this is needed or if PublicRead is being misued
		s3BucketSummariesPublicPolicy := &gocf.S3BucketPolicy{
			Bucket: gocf.Ref(connections.S3UploadBucketResourceName).String(),
			PolicyDocument: sparta.ArbitraryJSONObject{
				"Version": "2012-10-17",
				"Statement": []sparta.ArbitraryJSONObject{
					{
						"Sid":    "PublicReadGetObject",
						"Effect": "Allow",
						"Principal": sparta.ArbitraryJSONObject{
							"AWS": "*",
						},
						"Action": "s3:GetObject",
						"Resource": gocf.Join("",
							gocf.GetAtt(connections.S3UploadBucketResourceName, "Arn"),
							gocf.String("/*")),
						"Condition": sparta.ArbitraryJSONObject{
							"StringEquals": sparta.ArbitraryJSONObject{
								fmt.Sprintf("s3:ExistingObjectTag/%s", tagNameAccess): tagAccessPublic,
							},
						},
					},
				},
			},
		}
		s3PolicyResourceName := sparta.CloudFormationResourceName("S3BucketSummariesPublicPolicy",
			"S3BucketSummariesPublicPolicy")
		cfTemplate.AddResource(s3PolicyResourceName, s3BucketSummariesPublicPolicy)
		return nil
	}
}

////////////////////////////////////////////////////////////////////////////////
// Create
func (gws *ServicefulService) newOnS3PutGenerateSummary(api *sparta.API) *sparta.LambdaAWSInfo {
	// Privilege must include access to the S3 bucket for GetObjectRequest
	lambdaFn := sparta.HandleAWSLambda("GenerateSummary",
		gws.onS3PutGenerateSummary,
		sparta.IAMRoleDefinition{})
	lambdaFn.Options = &sparta.LambdaFunctionOptions{
		Description: "Produce a consolidated processing report",
		MemorySize:  256,
		Timeout:     10,
		TracingConfig: &gocf.LambdaFunctionTracingConfig{
			Mode: gocf.String("Active"),
		},
	}
	// IAM Role privileges
	lambdaFn.RoleDefinition.Privileges = gws.bucketGetPutPrivileges()

	// Dependency
	lambdaFn.DependsOn = []string{gws.connections.S3UploadBucketResourceName}

	// Event Triggers
	lambdaFn.Permissions = append(lambdaFn.Permissions,
		gws.s3NotificationPrefixBasedPermission(gws.connections.S3KeyspacePollyArtifacts))

	// Add the decorator so that the assets we publish are marked as public
	lambdaFn.Decorators = append(lambdaFn.Decorators,
		s3BucketTagPublicAccessDecorator(gws.connections))

	return lambdaFn
}
