package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type VPCResource struct {
	VPCId     string         `json:"vpc_id" dynamodbav:"vpc_id"`
	CreatedAt string         `json:"created_at" dynamodbav:"created_at"`
	CreatedBy string         `json:"created_by" dynamodbav:"created_by"`
	VPCCidr   string         `json:"vpc_cidr" dynamodbav:"vpc_cidr"`
	VPCName   string         `json:"vpc_name" dynamodbav:"vpc_name"`
	Status    string         `json:"status" dynamodbav:"status"`
	Subnets   []SubnetResult `json:"subnets" dynamodbav:"subnets"`
}

type SubnetResult struct {
	SubnetId         string `json:"subnet_id" dynamodbav:"subnet_id"`
	CIDRBlock        string `json:"cidr_block" dynamodbav:"cidr_block"`
	AvailabilityZone string `json:"availability_zone" dynamodbav:"availability_zone"`
	Name             string `json:"name" dynamodbav:"name"`
}

type ListVPCResponse struct {
	VPCs  []VPCResource `json:"vpcs"`
	Count int           `json:"count"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

var (
	dynamoClient *dynamodb.Client
	vpcTableName string
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(fmt.Sprintf("unable to load SDK config: %v", err))
	}

	dynamoClient = dynamodb.NewFromConfig(cfg)
	vpcTableName = os.Getenv("VPC_TABLE_NAME")
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	vpcId := request.PathParameters["vpc_id"]

	if vpcId != "" {
		return getVPC(ctx, vpcId)
	}

	return listVPCs(ctx, request.QueryStringParameters)
}

func getVPC(ctx context.Context, vpcId string) (events.APIGatewayProxyResponse, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(vpcTableName),
		KeyConditionExpression: aws.String("vpc_id = :vpc_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":vpc_id": &types.AttributeValueMemberS{Value: vpcId},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(1),
	}

	result, err := dynamoClient.Query(ctx, queryInput)
	if err != nil {
		return errorResponse(500, "Failed to query DynamoDB", err.Error())
	}

	if len(result.Items) == 0 {
		return errorResponse(404, "VPC not found", fmt.Sprintf("VPC with ID %s does not exist", vpcId))
	}

	var vpc VPCResource
	err = attributevalue.UnmarshalMap(result.Items[0], &vpc)
	if err != nil {
		return errorResponse(500, "Failed to parse VPC data", err.Error())
	}

	return successResponse(200, vpc)
}

func listVPCs(ctx context.Context, queryParams map[string]string) (events.APIGatewayProxyResponse, error) {
	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(vpcTableName),
	}

	result, err := dynamoClient.Scan(ctx, scanInput)
	if err != nil {
		return errorResponse(500, "Failed to scan DynamoDB", err.Error())
	}

	vpcs := make([]VPCResource, 0, len(result.Items))
	for _, item := range result.Items {
		var vpc VPCResource
		err := attributevalue.UnmarshalMap(item, &vpc)
		if err != nil {
			continue
		}
		vpcs = append(vpcs, vpc)
	}

	return successResponse(200, ListVPCResponse{
		VPCs:  vpcs,
		Count: len(vpcs),
	})
}

func successResponse(statusCode int, body interface{}) (events.APIGatewayProxyResponse, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return errorResponse(500, "Failed to marshal response", err.Error())
	}

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(bodyBytes),
	}, nil
}

func errorResponse(statusCode int, message, details string) (events.APIGatewayProxyResponse, error) {
	errorResp := ErrorResponse{
		Error:   message,
		Message: details,
	}
	bodyBytes, _ := json.Marshal(errorResp)

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(bodyBytes),
	}, nil
}

func main() {
	lambda.Start(handler)
}
