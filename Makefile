.PHONY: check-env \
	deploy \
	remove \
	package \
	precondition-aws \

precondition-aws:
	# PRECONDITION: AWS credentials available
	@echo
ifdef AWS_PROFILE
	@echo "CHECK: Make sure the profile '${AWS_PROFILE}' exists in `echo ~/.aws/credentials`)"
	# Troubleshooting: If it fails...
	# - Execute 'aws configure --profile ${AWS_PROFILE}'
	aws configure list --profile ${AWS_PROFILE}
	@echo
endif
	# Ensure the AWS credentials are set
	aws sts get-caller-identity
	@echo

package:
	sh scripts/package.sh
	@echo
	# GENERATING CloudFormation from Serverless
	serverless package
	@echo

deploy: precondition-aws
	# DEPLOYING to the cloud
	serverless deploy --force --verbose
	mkdir -p logs
	serverless info --verbose > logs/${DEPLOYMENT_NAME}-service-information.txt
	echo - See logs/${DEPLOYMENT_NAME}-service-information.txt

remove:
	# REMOVING cloud deployment
	serverless remove