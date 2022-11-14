terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-fixed.cfg launch cri-resmgr

# pod0, pod1, pod2, pod3, pod4, pod5, pod6, pod7: run on a different 7-cpu balloons.
CPUREQLIM="6500m- 100m-100m 100m-2000m"
INITCPUREQLIM="100m-100m 100m-100m 100m-100m"
POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: static"
n=8 create multicontainerpod
report allowed
verify 'disjoint_sets(cpus["pod0c0"], cpus["pod1c0"], cpus["pod2c0"], cpus["pod3c0"], cpus["pod4c0"], cpus["pod5c0"], cpus["pod6c0"], cpus["pod7c0"])' \
       'len(cpus["pod0c0"]) == 7' \
       'len(cpus["pod1c0"]) == 7' \
       'len(cpus["pod2c0"]) == 7' \
       'len(cpus["pod3c0"]) == 7' \
       'len(cpus["pod4c0"]) == 7' \
       'len(cpus["pod5c0"]) == 7' \
       'len(cpus["pod6c0"]) == 7' \
       'len(cpus["pod7c0"]) == 7'

# pod8: try to fit in the already full balloons
CPUREQLIM="6500m- 100m-100m 100m-2000m"
( POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: static" wait_t=30s create multicontainerpod) && {
    error "creating pod8 succeeded but was expected to fail with balloon allocation error"
}
vm-command "kubectl describe pod pod8"
      
if ! grep -q 'no suitable balloon instance available' <<< "$COMMAND_OUTPUT"; then
    error "could not find 'no suitable balloon instance available' in pod8 description"
fi
