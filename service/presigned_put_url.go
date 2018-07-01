package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	awsLambdaContext "github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
	spartaEvents "github.com/mweagle/Sparta/aws/events"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/sirupsen/logrus"
)

type presignedResponse struct {
	PresignedURL string `json:"put_object_url"`
	ResultsURL   string `json:"results_url"`
}

/*
================================================================================
╦  ╔═╗╔╦╗╔╗ ╔╦╗╔═╗
║  ╠═╣║║║╠╩╗ ║║╠═╣
╩═╝╩ ╩╩ ╩╚═╝═╩╝╩ ╩
================================================================================
Create a presigned URL
*/
func (gws *ServicefulService) s3GetPresignedURLLambda(ctx context.Context,
	apigRequest spartaEvents.APIGatewayRequest) (*presignedResponse, error) {
	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	lambdaContext, _ := awsLambdaContext.FromContext(ctx)

	discover, discoveryInfoErr := sparta.Discover()
	if discoveryInfoErr != nil {
		return nil, discoveryInfoErr
	}

	s3Resource := discover.Resources[gws.connections.S3UploadBucketResourceName]

	logger.WithFields(logrus.Fields{
		"RequestID":           lambdaContext.AwsRequestID,
		"S3Ref":               s3Resource.ResourceRef,
		"S3ResourcePoperties": s3Resource.Properties,
	}).Info("Request received")

	objectPath := fmt.Sprintf("%s/%s",
		gws.connections.S3KeyspaceUploads,
		lambdaContext.AwsRequestID)
	putObjectInput := &s3.PutObjectInput{
		Bucket: aws.String(s3Resource.ResourceRef),
		Key:    aws.String(objectPath),
	}
	awsSession := spartaAWS.NewSession(logger)
	s3svc := s3.New(awsSession)
	presignedReq, _ := s3svc.PutObjectRequest(putObjectInput)
	url, err := presignedReq.Presign(5 * time.Minute)
	if nil != err {
		return nil, err
	}
	resultsURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s/%s",
		s3Resource.ResourceRef,
		gws.connections.S3KeyspaceConsolidatedStatus,
		lambdaContext.AwsRequestID)

	return &presignedResponse{
		PresignedURL: url,
		ResultsURL:   resultsURL,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

// NewS3PresignedPutItemLambda defines a Lambda function that handles returning
// a presigned URL for upload
func (gws *ServicefulService) newS3PresignedPutItemLambda(api *sparta.API) *sparta.LambdaAWSInfo {
	// Register
	lambdaFn := sparta.HandleAWSLambda("PresignedURLProvider",
		gws.s3GetPresignedURLLambda,
		sparta.IAMRoleDefinition{})
	// IAM
	lambdaFn.RoleDefinition.Privileges = gws.bucketGetPutPrivileges()
	// X-Ray
	lambdaFn.Options.TracingConfig = &gocf.LambdaFunctionTracingConfig{
		Mode: gocf.String("Active"),
	}
	lambdaFn.DependsOn = []string{gws.connections.S3UploadBucketResourceName}
	if api != nil {
		apiGatewayResource, _ := api.NewResource("/presigned", lambdaFn)

		// We only return http.StatusOK
		apiMethod, apiMethodErr := apiGatewayResource.NewMethod("GET",
			http.StatusOK,
			http.StatusInternalServerError)
		if nil != apiMethodErr {
			panic("Failed to create /presigned resource: " + apiMethodErr.Error())
		}
		// The lambda resource only supports application/json Unmarshallable
		// requests.
		apiMethod.SupportedRequestContentTypes = []string{"application/json"}
	}
	return lambdaFn
}
