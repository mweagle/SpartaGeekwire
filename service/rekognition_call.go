package service

import (
	"context"
	"fmt"

	awsLamdaEvents "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rekognition"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

/*
================================================================================
╦  ╔═╗╔╦╗╔╗ ╔╦╗╔═╗
║  ╠═╣║║║╠╩╗ ║║╠═╣
╩═╝╩ ╩╩ ╩╚═╝═╩╝╩ ╩
================================================================================
Listen for PutObject events and submit the image to Rekognition
*/
func (gws *ServicefulService) onS3PutUploadEvent(ctx context.Context,
	s3Event awsLamdaEvents.S3Event) error {

	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	awsSession := spartaAWS.NewSession(logger)
	rekognitionSvc := rekognition.New(awsSession)

	handler := func(ctx context.Context,
		event awsLamdaEvents.S3EventRecord) (interface{}, error) {
		input := &rekognition.DetectLabelsInput{
			Image: &rekognition.Image{
				S3Object: &rekognition.S3Object{
					Bucket: aws.String(event.S3.Bucket.Name),
					Name:   aws.String(event.S3.Object.Key),
				},
			},
		}
		result, resultErr := rekognitionSvc.DetectLabels(input)
		if resultErr != nil {
			return nil, errors.Wrapf(resultErr, "Failed to detect text in image: %#v", input.Image.S3Object)
		}
		// So we only want the last part of the input key
		baseName := gws.baseKeyname(event.S3.Object.Key)
		keyPath := fmt.Sprintf("%s/%s",
			gws.connections.S3KeyspaceRekognitionArtifacts,
			baseName)
		putObjectResult := gws.putJSONObjectToS3(ctx,
			event.S3.Bucket.Name,
			keyPath,
			result,
			nil)
		if putObjectResult != nil {
			return nil, errors.Wrapf(putObjectResult, "Failed to put JSON response: %#v", keyPath)
		}
		logger.WithField("Result", *result).Info("Put Item")
		return nil, nil
	}
	handleResult, handleErr := gws.handleS3Records(ctx, s3Event, handler)
	logger.WithField("Results", handleResult).Info("S3 event results")
	return handleErr
}

////////////////////////////////////////////////////////////////////////////////
// Create
func (gws *ServicefulService) newOnPutCallRekognition(api *sparta.API) *sparta.LambdaAWSInfo {
	// Privilege must include access to the S3 bucket for GetObjectRequest
	lambdaFn := sparta.HandleAWSLambda("RekognitionRelay",
		gws.onS3PutUploadEvent,
		sparta.IAMRoleDefinition{})
	lambdaFn.Options = &sparta.LambdaFunctionOptions{
		Description: "Wait for the DetectText call to finish",
		MemorySize:  128,
		Timeout:     10,
		TracingConfig: &gocf.LambdaFunctionTracingConfig{
			Mode: gocf.String("Active"),
		},
	}
	// IAM Role privileges
	lambdaFn.RoleDefinition.Privileges = gws.bucketGetPutPrivileges("rekognition:DetectLabels")

	// Dependency
	lambdaFn.DependsOn = []string{gws.connections.S3UploadBucketResourceName}

	// Event Triggers
	lambdaFn.Permissions = append(lambdaFn.Permissions,
		gws.s3NotificationPrefixBasedPermission(gws.connections.S3KeyspaceUploads))

	return lambdaFn
}
