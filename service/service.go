package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	awsLamdaEvents "github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	sparta "github.com/mweagle/Sparta"
	spartaAWS "github.com/mweagle/Sparta/aws"
	spartaCF "github.com/mweagle/Sparta/aws/cloudformation"
	iamBuilder "github.com/mweagle/Sparta/aws/iam/builder"
	gocf "github.com/mweagle/go-cloudformation"
	ssmcache "github.com/mweagle/ssm-cache"
	"github.com/pkg/errors"
)

const (
	tagNameAccess   = "access"
	tagAccessPublic = "public"
)

// Connections is the type that defines the connections between
// the functions
type Connections struct {
	S3UploadBucketResourceName     string
	S3KeyspaceUploads              string
	S3KeyspaceRekognitionArtifacts string
	S3KeyspacePollyArtifacts       string
	S3KeyspaceComprehendArtifacts  string
	S3KeyspaceConsolidatedStatus   string
}
type recordHandler func(ctx context.Context, event awsLamdaEvents.S3EventRecord) (interface{}, error)

type s3Result struct {
	err error
	ret interface{}
}

// ServicefulService represents the lambda microservice of multiple
// functions cooperating to support a workflow
type ServicefulService struct {
	connections *Connections
	cacheClient ssmcache.Client
}

func (gws *ServicefulService) baseKeyname(s3Keypath string) string {
	keyParts := strings.Split(s3Keypath, "/")
	baseName := keyParts[len(keyParts)-1]
	return strings.TrimSuffix(baseName, path.Ext(baseName))
}

// getS3Object is a utility function to fet
func (gws *ServicefulService) getS3Object(ctx context.Context,
	bucket string,
	keyPath string) ([]byte, error) {

	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	awsSession := spartaAWS.NewSession(logger)
	s3Svc := s3.New(awsSession)

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(keyPath),
	}

	// Get the JSON output from Rekognition
	getResult, getResultErr := s3Svc.GetObject(getObjectInput)
	if getResultErr != nil {
		return nil, errors.Wrapf(getResultErr, "Failed to get object")
	}
	defer getResult.Body.Close()
	allData, allDataErr := ioutil.ReadAll(getResult.Body)
	if allDataErr != nil {
		return nil, errors.Wrapf(allDataErr, "Failed to read all data")
	}
	return allData, nil
}

func (gws *ServicefulService) putJSONObjectToS3(ctx context.Context,
	bucket string,
	keyPath string,
	data interface{},
	tags map[string]string) error {
	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)
	awsSession := spartaAWS.NewSession(logger)
	s3Svc := s3.New(awsSession)

	jsonData, jsonDataErr := json.Marshal(data)
	if jsonDataErr != nil {
		return errors.Wrapf(jsonDataErr,
			"Failed to marshal object to JSON for S3 storage")
	}

	// Put the item there...
	putObjectInput := &s3.PutObjectInput{
		Body:        aws.ReadSeekCloser(bytes.NewReader(jsonData)),
		Bucket:      aws.String(bucket),
		Key:         aws.String(keyPath),
		ContentType: aws.String("application/json"),
	}
	encodedTags := url.Values{}
	for eachKey, eachValue := range tags {
		encodedTags.Set(eachKey, eachValue)
	}
	if len(tags) != 0 {
		putObjectInput.Tagging = aws.String(encodedTags.Encode())
	}
	putResult, putResultErr := s3Svc.PutObject(putObjectInput)
	if putResultErr != nil {
		return errors.Wrapf(putResultErr, "Failed to put JSON response: %#v", putObjectInput)
	}
	logger.WithField("Result", *putResult).Debug("Put Item")
	return nil
}

// s3PubSubPrivileges is a shared function that returns the privileges necessary
// to use the S3 bucket as a pubsub creator
func (gws *ServicefulService) handleS3Records(ctx context.Context,
	s3Event awsLamdaEvents.S3Event,
	handler recordHandler) (interface{}, error) {
	logger, _ := ctx.Value(sparta.ContextKeyLogger).(*logrus.Logger)

	var wg sync.WaitGroup
	chanResults := make(chan s3Result, len(s3Event.Records))
	for _, eachRecord := range s3Event.Records {
		wg.Add(1)
		go func(event awsLamdaEvents.S3EventRecord) {
			ret, err := handler(ctx, event)
			handlerResult := s3Result{
				err: err,
				ret: ret,
			}
			wg.Done()
			logger.WithField("RESULT", fmt.Sprintf("%#v", handlerResult)).Info("Task complete")
			chanResults <- handlerResult
		}(eachRecord)
	}
	logger.Info("Waiting")
	wg.Wait()
	logger.Info("Done waiting")
	close(chanResults)
	// Accumulate all the results
	handlerErrors := make([]error, 0)
	handled := 0
	for eachEvent := range chanResults {
		if eachEvent.err != nil {
			handlerErrors = append(handlerErrors, eachEvent.err)
		} else {
			handled++
		}
	}
	logger.Info("Done reading results")

	if len(handlerErrors) != 0 {
		return nil, errors.Errorf("Failed to process: %#v", handlerErrors)
	}
	return fmt.Sprintf("Processed %d events", handled), nil
}

// s3NotificationPrefixFilter is a DRY spec for setting up a notification configuration
// filter
func (gws *ServicefulService) s3NotificationPrefixBasedPermission(keyPathPrefix string) sparta.S3Permission {
	return sparta.S3Permission{
		BasePermission: sparta.BasePermission{
			SourceArn: gocf.Ref(gws.connections.S3UploadBucketResourceName),
		},
		Events: []string{"s3:ObjectCreated:*"},
		Filter: s3.NotificationConfigurationFilter{
			Key: &s3.KeyFilter{
				FilterRules: []*s3.FilterRule{
					&s3.FilterRule{
						Name:  aws.String("prefix"),
						Value: aws.String(keyPathPrefix),
					},
				},
			},
		},
	}
}

// s3PubSubPrivileges is a shared function that returns the privileges necessary
// to use the S3 bucket as a pubsub creator. resourceUnscopedActions is an
// optional set of actions to allow that don't use resource-based
// permissions
func (gws *ServicefulService) bucketGetPutPrivileges(resourceUnscopedActions ...string) []sparta.IAMRolePrivilege {
	privileges := []sparta.IAMRolePrivilege{
		sparta.IAMRolePrivilege{
			Actions: []string{"s3:GetObject",
				"s3:PutObject",
				"s3:PutObjectTagging"},
			Resource: spartaCF.S3AllKeysArnForBucket(gocf.Ref(gws.connections.S3UploadBucketResourceName)),
		},
		iamBuilder.Allow("ssm:GetParameter", "ssm:GetParametersByPath").
			ForResource().
			Literal("arn:aws:ssm:").
			Region(":").
			AccountID(":").
			Literal("*").
			ToPrivilege(),
	}

	if len(resourceUnscopedActions) != 0 {
		privileges = append(privileges, sparta.IAMRolePrivilege{
			Actions:  resourceUnscopedActions,
			Resource: "*",
		})
	}
	return privileges
}

// New returns a service that stitches multiple lambdas into
// a single workflow...
func New(connections *Connections, api *sparta.API) []*sparta.LambdaAWSInfo {
	gws := &ServicefulService{
		connections: connections,
		cacheClient: ssmcache.NewClient(5 * time.Minute),
	}
	var lambdaFunctions []*sparta.LambdaAWSInfo
	lambdaFunctions = append(lambdaFunctions, gws.newS3PresignedPutItemLambda(api))
	lambdaFunctions = append(lambdaFunctions, gws.newOnPutCallRekognition(api))
	lambdaFunctions = append(lambdaFunctions, gws.newOnS3PutCallPolly(api))
	lambdaFunctions = append(lambdaFunctions, gws.newOnS3PutGenerateSummary(api))

	lambdaFunctions = append(lambdaFunctions, gws.newOnFeedbackDetectSentiment(api))

	// Add a decorator to ensure that the bucket that we're using has the proper
	// policy set so that the "summaries" are publicly available

	return lambdaFunctions
}
