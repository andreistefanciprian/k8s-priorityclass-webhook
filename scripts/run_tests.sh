	kustomize build infra/test | kubectl apply -f -
    
    echo
	echo "Test 1 - Checking that a Pod doesn't get scheduled without a priorityClassName"
	kubectl get pod/test-1 -n boo -o yaml | grep priority
	
    echo
	echo "Test 2 - Checking that a Deployment gets scheduled with a priorityClassName mutated by webhook"
	kubectl get deployment/test-2 -n boo -o yaml | grep priority
	
    echo
	echo "Test 3 - Checking that a Deployment that has priorityClassName set gets scheduled with priorityClassName mutated by webhook"
	kubectl get deployment/test-3 -n boo -o yaml | grep priority