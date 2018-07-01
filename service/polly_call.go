package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	awsLamdaEvents "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/polly"
	"github.com/aws/aws-sdk-go/service/rekognition"
	"github.com/aws/aws-sdk-go/service/s3"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
	gocf "github.com/mweagle/go-cloudformation"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	defaultPollyVoice = "Joanna"
	pollySSMLTemplate = `<speak>
It appears that your image includes the text quote <break time="1s"/>
<prosody pitch="x-high"> %s </prosody> endquote <break time="1s"/> with a confidence of %.2f percent.
</speak>`
	pollySSMLLabelTemplate = `<speak>
It appears that this image includes <amazon:breath/><break time="1s"/><emphasis>
 %s</emphasis><break time="1s"/> with a confidence of %.2f percent.
</speak>`
)

/*
================================================================================
╦  ╔═╗╔╦╗╔╗ ╔╦╗╔═╗
║  ╠═╣║║║╠╩╗ ║║╠═╣
╩═╝╩ ╩╩ ╩╚═╝═╩╝╩ ╩
================================================================================
Listen for PutObject events and submit the image to Rekognition
*/
func (gws *ServicefulService) onS3PutCallPolly(ctx context.Context, s3Event awsLamdaEvents.S3Event) error {
	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	awsSession := spartaAWS.NewSession(logger)
	s3Svc := s3.New(awsSession)
	pollySvc := polly.New(awsSession)
	pollyVoice, _ := gws.cacheClient.GetExpiringString("/SpartaPollyWorkflow/VoiceId",
		30*time.Second)
	if pollyVoice == "" {
		pollyVoice = defaultPollyVoice
	}
	// Process all the events...
	handler := func(ctx context.Context, event awsLamdaEvents.S3EventRecord) (interface{}, error) {

		// Get the JSON output from Rekognition
		rekognitionData, rekognitionDataErr := gws.getS3Object(ctx, event.S3.Bucket.Name, event.S3.Object.Key)
		if rekognitionDataErr != nil {
			return nil, rekognitionDataErr
		}
		rekognitionResponse := rekognition.DetectLabelsOutput{}
		unmarshalErr := json.Unmarshal(rekognitionData, &rekognitionResponse)
		if unmarshalErr != nil {
			return nil, unmarshalErr
		}
		textType := "text"
		synthesizeText := "I'm afraid I didn't find anything in your image"
		currentScore := float64(0.0)
		for _, eachLabel := range rekognitionResponse.Labels {
			if *eachLabel.Confidence >= currentScore {
				synthesizeText = fmt.Sprintf(pollySSMLLabelTemplate,
					*eachLabel.Name,
					*eachLabel.Confidence)
				currentScore = *eachLabel.Confidence
				textType = "ssml"
			}
		}
		// Super send it to polly
		pollyInput := polly.SynthesizeSpeechInput{
			Text:         aws.String(synthesizeText),
			TextType:     aws.String(textType),
			OutputFormat: aws.String("mp3"),
			VoiceId:      aws.String(pollyVoice),
		}
		pollyOutput, pollyOutputErr := pollySvc.SynthesizeSpeech(&pollyInput)
		if pollyOutputErr != nil {
			return nil, pollyOutputErr
		}
		// Winning, write it back to the location...
		// So we only want the last part of the input key
		baseName := gws.baseKeyname(event.S3.Object.Key)

		// Copy the response to a /tmp file
		outputFilePath := filepath.Join("/tmp", baseName)
		outputFileHandle, outputFileHandleErr := os.Create(outputFilePath)
		if outputFileHandleErr != nil {
			return nil, outputFileHandleErr
		}
		defer outputFileHandle.Close()
		defer os.Remove(outputFilePath)
		_, copyErr := io.Copy(outputFileHandle, pollyOutput.AudioStream)
		if copyErr != nil {
			return nil, nil
		}
		outputFileHandle.Seek(0, 0)

		// Save it to the other location
		putObjectInput := &s3.PutObjectInput{
			Body:   outputFileHandle,
			Bucket: aws.String(event.S3.Bucket.Name),
			Key: aws.String(fmt.Sprintf("%s/%s",
				gws.connections.S3KeyspacePollyArtifacts,
				baseName)),
			ContentType: aws.String("audio/mpeg3"),
		}
		defer pollyOutput.AudioStream.Close()

		putResult, putResultErr := s3Svc.PutObject(putObjectInput)
		if putResultErr != nil {
			return nil, errors.Wrapf(putResultErr, "Failed to put mp3 response: %#v", putObjectInput)
		}
		logger.WithField("Result", *putResult).Info("Put Item")
		return nil, nil
	}
	handleResult, handleErr := gws.handleS3Records(ctx, s3Event, handler)
	logger.WithField("Results", handleResult).Info("S3 event results")
	return handleErr
}

////////////////////////////////////////////////////////////////////////////////
// Create
func (gws *ServicefulService) newOnS3PutCallPolly(api *sparta.API) *sparta.LambdaAWSInfo {
	// Privilege must include access to the S3 bucket for GetObjectRequest
	lambdaFn := sparta.HandleAWSLambda("PollyRelay",
		gws.onS3PutCallPolly,
		sparta.IAMRoleDefinition{})
	lambdaFn.Options = &sparta.LambdaFunctionOptions{
		Description: "Take the most confidently detected text and synthesize it",
		MemorySize:  128,
		Timeout:     10,
		TracingConfig: &gocf.LambdaFunctionTracingConfig{
			Mode: gocf.String("Active"),
		},
	}
	// IAM Role privileges
	lambdaFn.RoleDefinition.Privileges = gws.bucketGetPutPrivileges("polly:SynthesizeSpeech")

	// Event Triggers
	lambdaFn.Permissions = append(lambdaFn.Permissions,
		gws.s3NotificationPrefixBasedPermission(gws.connections.S3KeyspaceRekognitionArtifacts))

	// Dependency
	lambdaFn.DependsOn = []string{gws.connections.S3UploadBucketResourceName}

	return lambdaFn
}
