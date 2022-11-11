terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-multicontainer.cfg cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr

POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: bln0"
CPUREQLIM="50m- 50m- 3000m-4000m"
create multicontainer
report allowed
