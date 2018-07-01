package service

import (
	"context"
	"fmt"
	"net/http"

	awsLambdaContext "github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/comprehend"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
	spartaEvents "github.com/mweagle/Sparta/aws/events"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/sirupsen/logrus"
)

// FeedbackBody is the typed body submitted in a FeedbackRequest
type FeedbackBody struct {
	Language string `json:"lang"`
	Comment  string `json:"comment"`
}

// FeedbackRequest is the typed input to the
// onFeedbackDetectSentiment
type FeedbackRequest struct {
	spartaEvents.APIGatewayEnvelope
	Body FeedbackBody `json:"body"`
}

// FeedbackResponse is the response type sent back in response to
// a sentiment analysis
type FeedbackResponse struct {
	Sentiment *comprehend.DetectSentimentOutput `json:"sentiment"`
	Comment   string                            `json:"comment"`
}

/*
================================================================================
╦  ╔═╗╔╦╗╔╗ ╔╦╗╔═╗
║  ╠═╣║║║╠╩╗ ║║╠═╣
╩═╝╩ ╩╩ ╩╚═╝═╩╝╩ ╩
================================================================================
Create a presigned URL
*/
func (gws *ServicefulService) onFeedbackDetectSentiment(ctx context.Context,
	apigRequest FeedbackRequest) (*FeedbackResponse, error) {

	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	lambdaContext, _ := awsLambdaContext.FromContext(ctx)
	discover, discoveryInfoErr := sparta.Discover()
	if discoveryInfoErr != nil {
		return nil, discoveryInfoErr
	}
	s3Resource := discover.Resources[gws.connections.S3UploadBucketResourceName]
	comment := apigRequest.Body.Comment
	language := apigRequest.Body.Language
	if language == "" {
		language = "en"
	}
	awsSession := spartaAWS.NewSession(logger)
	svcComprehend := comprehend.New(awsSession)
	detectSentimentInput := &comprehend.DetectSentimentInput{
		LanguageCode: aws.String(language),
		Text:         aws.String(comment),
	}
	detectSentimentResult, detectSentimentResultErr := svcComprehend.DetectSentiment(detectSentimentInput)
	if detectSentimentResultErr != nil {
		return nil, detectSentimentResultErr
	}
	response := &FeedbackResponse{
		Sentiment: detectSentimentResult,
		Comment:   comment}

	// Super, save this to a location...
	outputKey := fmt.Sprintf("%s/%s.json",
		gws.connections.S3KeyspaceComprehendArtifacts,
		lambdaContext.AwsRequestID)
	putErr := gws.putJSONObjectToS3(ctx,
		s3Resource.ResourceRef,
		outputKey,
		response,
		nil)
	if putErr != nil {
		return nil, putErr
	}
	return response, nil
}

////////////////////////////////////////////////////////////////////////////////

// NewS3PresignedPutItemLambda defines a Lambda function that handles returning
// a presigned URL for upload
func (gws *ServicefulService) newOnFeedbackDetectSentiment(api *sparta.API) *sparta.LambdaAWSInfo {
	// Privilege must include access to the S3 bucket for GetObjectRequest
	lambdaFn := sparta.HandleAWSLambda("FeedbackDetectSentiment",
		gws.onFeedbackDetectSentiment,
		sparta.IAMRoleDefinition{})
	lambdaFn.Options.TracingConfig = &gocf.LambdaFunctionTracingConfig{
		Mode: gocf.String("Active"),
	}

	lambdaFn.RoleDefinition.Privileges = gws.bucketGetPutPrivileges("comprehend:DetectSentiment")

	lambdaFn.DependsOn = []string{gws.connections.S3UploadBucketResourceName}
	if api != nil {
		apiGatewayResource, _ := api.NewResource("/feedback", lambdaFn)

		// We only return http.StatusOK
		apiMethod, apiMethodErr := apiGatewayResource.NewMethod("POST",
			http.StatusOK,
			http.StatusInternalServerError)
		if nil != apiMethodErr {
			panic("Failed to create /feedback resource: " + apiMethodErr.Error())
		}
		// The lambda resource only supports application/json Unmarshallable
		// requests.
		apiMethod.SupportedRequestContentTypes = []string{"application/json"}
	}
	return lambdaFn
}
