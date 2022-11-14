terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-dynamic.cfg cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr

# pod0: create dynamically the first dynamic balloon
CPUREQLIM="6500m- 100m-100m 100m-2000m"
INITCPUREQLIM="100m-100m 100m-100m 100m-100m"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
create multicontainerpod
report allowed
verify 'len(cpus["pod0c0"]) == 7' \
       'cpus["pod0c0"] == cpus["pod0c1"] == cpus["pod0c2"]' \
       'len(nodes["pod0c0"]) == 1' \
       'nodes["pod0c0"] == nodes["pod0c1"] == nodes["pod0c2"]'
verify-metrics-has-line 'balloon="dynamic\[0\]"'
verify-metrics-has-line 'balloon="dynamic\[0\]".* 7'
verify-metrics-has-no-line 'balloon="dynamic\[1\]"'
verify-metrics-has-line 'dynamic\[0\].*containers="pod0:pod0c0,pod0:pod0c1,pod0:pod0c2"'

# pod1: create dynamically the second dynamic balloon
CPUREQLIM="6500m- 100m-100m 100m-2000m"
INITCPUREQLIM="100m-100m 100m-100m 100m-100m"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
create multicontainerpod
report allowed
verify 'len(cpus["pod0c0"]) == 7' \
       'len(cpus["pod1c0"]) == 7' \
       'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"])' \
       'len(nodes["pod1c0"]) == 1'
verify-metrics-has-line 'balloon="dynamic\[1\]"'
verify-metrics-has-line 'balloon="dynamic\[1\]".* 7'

# check deflating a balloon in metrics
kubectl delete pods --now pod1
CPUREQLIM="2000m-"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
create multicontainerpod
verify-metrics-has-line 'balloon="dynamic\[1\]".* 2'
kubectl delete pods --now pod2

# pod3, pod4, pod5, pod6, pod7, pod8, pod9: create new balloons
CPUREQLIM="6500m- 100m-100m 100m-2000m"
INITCPUREQLIM="100m-100m 100m-100m 100m-100m"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic"
n=7 create multicontainerpod
report allowed
verify 'len(cpus["pod0c0"]) == 7' \
       'len(cpus["pod3c0"]) == 7' \
       'len(cpus["pod4c0"]) == 7' \
       'len(cpus["pod5c0"]) == 7' \
       'len(cpus["pod6c0"]) == 7' \
       'len(cpus["pod7c0"]) == 7' \
       'len(cpus["pod8c0"]) == 7' \
       'len(cpus["pod9c0"]) == 7'

# pod10: We ran out of dynamic ballons that can host this pod
CPUREQLIM="6500m- 100m-100m 100m-2000m"
INITCPUREQLIM="100m-100m 100m-100m 100m-100m"
( POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: dynamic" wait_t=30s create multicontainerpod ) && {
    error "creating pod10 succeeded but was expected to fail with balloon allocation error"
}
echo "pod8 creation failed with an error as expected"
vm-command "kubectl describe pod pod10"
if ! grep -q 'no suitable balloon instance available' <<< "$COMMAND_OUTPUT"; then
    error "could not find 'no suitable balloon instance available' in pod10 description"
fi
