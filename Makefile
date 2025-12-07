.PHONY: check-env \
	deploy \
	remove \
	package \
	clean \
	check-env \
	precondition-aws \

check-env:
	# PRECONDITION: Ensure to execute 'source env.local.sh' before!
	@echo
	ifndef DEPLOYMENT_NAME
		$(error environment variable DEPLOYMENT_NAME is undefined)
	endif

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

package: check-env
	@echo
	# GENERATING CloudFormation from Serverless
	serverless package
	@echo
	