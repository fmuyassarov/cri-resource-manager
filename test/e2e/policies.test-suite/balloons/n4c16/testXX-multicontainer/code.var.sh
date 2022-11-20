terminate cri-resmgr
cri_resmgr_cfg=${TEST_DIR}/balloons-multicontainer.cfg cri_resmgr_extra_args="-metrics-interval 4s" launch cri-resmgr

POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: bln0"
CPUREQLIM="50m- 150m-250m 3000m-5000m"
create multicontainer
report allowed

POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: bln0"
CPUREQLIM="1000m-2000m 300m-300m"
create multicontainer
report allowed

POD_ANNOTATION="balloon.balloons.cri-resource-manager.intel.com: bln0"
CPUREQLIM="1000m-2000m"
create multicontainer
report allowed
