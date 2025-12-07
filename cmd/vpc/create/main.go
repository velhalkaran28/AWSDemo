package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type CreateVPCRequest struct {
	CIDRBlock string   `json:"cidr_block"`
	VPCName   string   `json:"vpc_name"`
	Subnets   []Subnet `json:"subnets"`
}

type Subnet struct {
	CIDRBlock        string `json:"cidr_block"`
	Name             string `json:"name"`
	AvailabilityZone string `json:"availability_zone,omitempty"`
}

type CreateVPCResponse struct {
	Message   string         `json:"message"`
	VPCId     string         `json:"vpc_id"`
	VPCCidr   string         `json:"vpc_cidr"`
	Subnets   []SubnetResult `json:"subnets"`
	CreatedAt string         `json:"created_at"`
	CreatedBy string         `json:"created_by"`
}

type SubnetResult struct {
	SubnetId         string `json:"subnet_id"`
	CIDRBlock        string `json:"cidr_block"`
	AvailabilityZone string `json:"availability_zone"`
	Name             string `json:"name"`
}

// Error response structure
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

var (
	ec2Client    *ec2.Client
	dynamoClient *dynamodb.Client
	vpcTableName string
)

func init() {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		panic(fmt.Sprintf("unable to load SDK config: %v", err))
	}

	ec2Client = ec2.NewFromConfig(cfg)
	dynamoClient = dynamodb.NewFromConfig(cfg)
	vpcTableName = os.Getenv("VPC_TABLE_NAME")
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var vpcRequest CreateVPCRequest
	if err := json.Unmarshal([]byte(request.Body), &vpcRequest); err != nil {
		return errorResponse(400, "Invalid JSON", err.Error())
	}

	if err := validateInput(vpcRequest); err != nil {
		return errorResponse(400, "Validation failed", err.Error())
	}

	apiKey := request.Headers["x-api-key"]
	if apiKey == "" {
		apiKey = "api-user"
	}

	if len(apiKey) > 20 {
		apiKey = apiKey[:20]
	}

	vpcId, err := createVPC(ctx, vpcRequest)
	if err != nil {
		return errorResponse(500, "Failed to create VPC", err.Error())
	}

	subnetResults, err := createSubnets(ctx, vpcId, vpcRequest.Subnets)
	if err != nil {
		return errorResponse(500, "Failed to create subnets", err.Error())
	}

	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := storeVPCMetadata(ctx, vpcId, vpcRequest, subnetResults, createdAt, apiKey); err != nil {
		return errorResponse(500, "Failed to store metadata", err.Error())
	}

	response := CreateVPCResponse{
		Message:   "VPC created successfully",
		VPCId:     vpcId,
		VPCCidr:   vpcRequest.CIDRBlock,
		Subnets:   subnetResults,
		CreatedAt: createdAt,
		CreatedBy: apiKey,
	}

	return successResponse(201, response)
}

func validateInput(req CreateVPCRequest) error {
	if req.CIDRBlock == "" {
		return fmt.Errorf("cidr_block is required")
	}
	if req.VPCName == "" {
		return fmt.Errorf("vpc_name is required")
	}
	if len(req.Subnets) == 0 {
		return fmt.Errorf("at least one subnet is required")
	}
	for i, subnet := range req.Subnets {
		if subnet.CIDRBlock == "" {
			return fmt.Errorf("subnet[%d]: cidr_block is required", i)
		}
		if subnet.Name == "" {
			return fmt.Errorf("subnet[%d]: name is required", i)
		}
	}
	return nil
}

func createVPC(ctx context.Context, req CreateVPCRequest) (string, error) {
	createVPCInput := &ec2.CreateVpcInput{
		CidrBlock: aws.String(req.CIDRBlock),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeVpc,
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String(req.VPCName)},
					{Key: aws.String("ManagedBy"), Value: aws.String("VPC-Management-API")},
				},
			},
		},
	}

	createVPCOutput, err := ec2Client.CreateVpc(ctx, createVPCInput)
	if err != nil {
		return "", fmt.Errorf("failed to create VPC: %v", err)
	}

	vpcId := aws.ToString(createVPCOutput.Vpc.VpcId)

	return vpcId, nil
}

func createSubnets(ctx context.Context, vpcId string, subnets []Subnet) ([]SubnetResult, error) {
	results := make([]SubnetResult, 0, len(subnets))

	azOutput, err := ec2Client.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("state"),
				Values: []string{"available"},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe availability zones: %w", err)
	}

	availableAZs := azOutput.AvailabilityZones

	for i, subnet := range subnets {

		var az string
		if subnet.AvailabilityZone != "" {
			az = subnet.AvailabilityZone
		} else if i < len(availableAZs) {
			az = aws.ToString(availableAZs[i].ZoneName)
		} else {
			az = aws.ToString(availableAZs[0].ZoneName)
		}

		createSubnetInput := &ec2.CreateSubnetInput{
			VpcId:            aws.String(vpcId),
			CidrBlock:        aws.String(subnet.CIDRBlock),
			AvailabilityZone: aws.String(az),
			TagSpecifications: []ec2types.TagSpecification{
				{
					ResourceType: ec2types.ResourceTypeSubnet,
					Tags: []ec2types.Tag{
						{Key: aws.String("Name"), Value: aws.String(subnet.Name)},
						{Key: aws.String("ManagedBy"), Value: aws.String("VPC-Management-API")},
					},
				},
			},
		}

		createSubnetOutput, err := ec2Client.CreateSubnet(ctx, createSubnetInput)
		if err != nil {
			return nil, fmt.Errorf("failed to create subnet %s: %w", subnet.Name, err)
		}

		results = append(results, SubnetResult{
			SubnetId:         aws.ToString(createSubnetOutput.Subnet.SubnetId),
			CIDRBlock:        subnet.CIDRBlock,
			AvailabilityZone: az,
			Name:             subnet.Name,
		})
	}

	return results, nil
}

func storeVPCMetadata(ctx context.Context, vpcId string, req CreateVPCRequest, subnets []SubnetResult, createdAt, createdBy string) error {
	subnetItems := make([]types.AttributeValue, 0, len(subnets))
	for _, subnet := range subnets {
		subnetItems = append(subnetItems, &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"subnet_id":         &types.AttributeValueMemberS{Value: subnet.SubnetId},
				"cidr_block":        &types.AttributeValueMemberS{Value: subnet.CIDRBlock},
				"availability_zone": &types.AttributeValueMemberS{Value: subnet.AvailabilityZone},
				"name":              &types.AttributeValueMemberS{Value: subnet.Name},
			},
		})
	}

	item := map[string]types.AttributeValue{
		"vpc_id":     &types.AttributeValueMemberS{Value: vpcId},
		"created_at": &types.AttributeValueMemberS{Value: createdAt},
		"created_by": &types.AttributeValueMemberS{Value: createdBy},
		"vpc_cidr":   &types.AttributeValueMemberS{Value: req.CIDRBlock},
		"vpc_name":   &types.AttributeValueMemberS{Value: req.VPCName},
		"status":     &types.AttributeValueMemberS{Value: "created"},
		"subnets":    &types.AttributeValueMemberL{Value: subnetItems},
	}

	_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(vpcTableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to store VPC metadata: %w", err)
	}

	return nil
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
