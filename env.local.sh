#!/usr/bin/env bash

export AWS_REGION="eu-west-1"
export AWS_ACCOUNT_ID=794785008288
export DEPLOYMENT_NAME=${DEPLOYMENT_NAME:=$USER} # Unix
export LOG_RETENTION_DAYS=7
export DEPLOYMENT_BUCKET_NAME=serverlessdeployment-karanv-demo
