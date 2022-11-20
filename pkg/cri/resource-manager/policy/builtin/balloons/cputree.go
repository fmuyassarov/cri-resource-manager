// Copyright 2022 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package balloons

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type CPUTopologyLevel int

const (
	CPUTopologyLevelUndefined CPUTopologyLevel = iota
	CPUTopologyLevelSystem
	CPUTopologyLevelPackage
	CPUTopologyLevelDie
	CPUTopologyLevelNuma
	CPUTopologyLevelCore
	CPUTopologyLevelThread
)

type cpuTreeAllocator struct {
	options cpuTreeAllocatorOptions
	root    *cpuTreeNode
}

type cpuTreeAllocatorOptions struct {
	topologyBalancing bool
}

type cpuTreeNode struct {
	name     string
	level    CPUTopologyLevel
	parent   *cpuTreeNode
	children []*cpuTreeNode
	cpus     cpuset.CPUSet
}

func NewCpuTreeFromSystem() (*cpuTreeNode, error) {
	sys, err := system.DiscoverSystem(system.DiscoverCPUTopology)
	if err != nil {
		return nil, err
	}
	sysTree := NewCpuTree("system")
	sysTree.level = CPUTopologyLevelSystem
	for _, packageID := range sys.PackageIDs() {
		packageTree := NewCpuTree(fmt.Sprintf("p%d", packageID))
		packageTree.level = CPUTopologyLevelPackage
		cpuPackage := sys.Package(packageID)
		sysTree.AddChild(packageTree)
		for _, dieID := range cpuPackage.DieIDs() {
			dieTree := NewCpuTree(fmt.Sprintf("p%dd%d", packageID, dieID))
			dieTree.level = CPUTopologyLevelDie
			packageTree.AddChild(dieTree)
			for _, nodeID := range cpuPackage.DieNodeIDs(dieID) {
				nodeTree := NewCpuTree(fmt.Sprintf("p%dd%dn%d", packageID, dieID, nodeID))
				nodeTree.level = CPUTopologyLevelNuma
				dieTree.AddChild(nodeTree)
				node := sys.Node(nodeID)
				for _, cpuID := range node.CPUSet().ToSlice() {
					cpuTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dcpu%d", packageID, dieID, nodeID, cpuID))

					cpuTree.level = CPUTopologyLevelCore
					nodeTree.AddChild(cpuTree)
					cpu := sys.CPU(cpuID)
					for _, threadID := range cpu.ThreadCPUSet().ToSlice() {
						threadTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dcpu%dt%d", packageID, dieID, nodeID, cpuID, threadID))
						threadTree.level = CPUTopologyLevelThread
						cpuTree.AddChild(threadTree)
						threadTree.AddCpus(cpuset.NewCPUSet(threadID))
					}
				}
			}
		}
	}
	return sysTree, nil
}

func NewCpuTree(name string) *cpuTreeNode {
	return &cpuTreeNode{
		name: name,
		cpus: cpuset.NewCPUSet(),
	}
}

func (t *cpuTreeNode) AddChild(child *cpuTreeNode) {
	child.parent = t
	t.children = append(t.children, child)
}

func (t *cpuTreeNode) AddCpus(cpus cpuset.CPUSet) {
	t.cpus = t.cpus.Union(cpus)
	if t.parent != nil {
		t.parent.AddCpus(cpus)
	}
}

func (t *cpuTreeNode) Cpus() cpuset.CPUSet {
	return t.cpus
}

func (t *cpuTreeNode) String() string {
	if len(t.children) == 0 {
		return t.name
	}
	return fmt.Sprintf("%s%v", t.name, t.children)
}

func (t *cpuTreeNode) NewAllocator(options cpuTreeAllocatorOptions) *cpuTreeAllocator {
	ta := &cpuTreeAllocator{
		root:    t,
		options: options,
	}
	return ta
}

var WalkSkipChildren error = errors.New("skip children")
var WalkStop error = errors.New("stop")

func (t *cpuTreeNode) DepthFirstWalk(handler func(*cpuTreeNode) error) error {
	if err := handler(t); err != nil {
		if err == WalkSkipChildren {
			return nil
		}
		return err
	}
	for _, child := range t.children {
		if err := child.DepthFirstWalk(handler); err != nil {
			return err
		}
	}
	return nil
}

func (ta *cpuTreeAllocator) sorterAllocate(tnas []cpuTreeNodeAttributes) func(int, int) bool {
	return func(i, j int) bool {
		if tnas[i].depth != tnas[j].depth {
			return tnas[i].depth > tnas[j].depth
		}
		for tdepth := 0; tdepth < len(tnas[i].currentCpuCounts); tdepth += 1 {
			// After this currentCpus will increase.
			// Maximize the maximal amount of currentCpus
			// as high level in the topology as possible.
			if tnas[i].currentCpuCounts[tdepth] != tnas[j].currentCpuCounts[tdepth] {
				return tnas[i].currentCpuCounts[tdepth] > tnas[j].currentCpuCounts[tdepth]
			}
		}
		for tdepth := 0; tdepth < len(tnas[i].freeCpuCounts); tdepth += 1 {
			// After this freeCpus will decrease.
			if tnas[i].freeCpuCounts[tdepth] != tnas[j].freeCpuCounts[tdepth] {
				if ta.options.topologyBalancing {
					// Goal: minimize maximal freeCpus in topology.
					return tnas[i].freeCpuCounts[tdepth] > tnas[j].freeCpuCounts[tdepth]
				} else {
					// Goal: maximize maximal freeCpus in topology.
					return tnas[i].freeCpuCounts[tdepth] < tnas[j].freeCpuCounts[tdepth]
				}
			}
		}
		return i > j
	}
}

func (ta *cpuTreeAllocator) sorterRelease(tnas []cpuTreeNodeAttributes) func(int, int) bool {
	return func(i, j int) bool {
		if tnas[i].depth != tnas[j].depth {
			return tnas[i].depth > tnas[j].depth
		}
		for tdepth := 0; tdepth < len(tnas[i].currentCpuCounts); tdepth += 1 {
			// After this currentCpus will decrease. Aim
			// to minimize the minimal amount of
			// currentCpus in order to decrease
			// fragmentation as high level in the topology
			// as possible.
			if tnas[i].currentCpuCounts[tdepth] != tnas[j].currentCpuCounts[tdepth] {
				return tnas[i].currentCpuCounts[tdepth] < tnas[j].currentCpuCounts[tdepth]
			}
		}
		for tdepth := 0; tdepth < len(tnas[i].freeCpuCounts); tdepth += 1 {
			// After this freeCpus will increase. Try to
			// maximize minimal free CPUs for better
			// isolation as high level in the topology as
			// possible.
			if tnas[i].freeCpuCounts[tdepth] != tnas[j].freeCpuCounts[tdepth] {
				if ta.options.topologyBalancing {
					return tnas[i].freeCpuCounts[tdepth] < tnas[j].freeCpuCounts[tdepth]
				} else {
					return tnas[i].freeCpuCounts[tdepth] < tnas[j].freeCpuCounts[tdepth]
				}
			}
		}
		return i < j
	}
}

// ResizeCpus returns two sets of CPUs: addFromCpus, removeFromCpus.
//   - addFromCpus contains free CPUs from which delta CPUs
//     can be allocated.
//   - removeFromCpus contains CPUs in currentCpus set from which delta CPUs
//     can be freed.
func (ta *cpuTreeAllocator) ResizeCpus(currentCpus, freeCpus cpuset.CPUSet, delta int) (cpuset.CPUSet, cpuset.CPUSet, error) {
	if delta > 0 {
		return ta.resizeCpus(currentCpus, freeCpus, delta)
	}
	// In multi-CPU removal, remove CPUs one by one instead of
	// trying to find a single topology element from which all of
	// them could be removed.
	removeFrom := cpuset.NewCPUSet()
	addFrom := cpuset.NewCPUSet()
	for n := 0; n < -delta; n++ {
		_, removeSingleFrom, err := ta.resizeCpus(currentCpus, freeCpus, -1)
		if err != nil {
			return addFrom, removeFrom, err
		}
		// Make cheap internal error checks in order to capture
		// issues in alternative algorithms.
		if removeSingleFrom.Size() != 1 {
			return addFrom, removeFrom, fmt.Errorf("internal error: failed to find single cpu to free, "+
				"currentCpus=%s freeCpus=%s expectedSingle=%s",
				currentCpus, freeCpus, removeSingleFrom)
		}
		if removeFrom.Union(removeSingleFrom).Size() != n+1 {
			return addFrom, removeFrom, fmt.Errorf("internal error: double release of a cpu, "+
				"currentCpus=%s freeCpus=%s alreadyRemoved=%s removedNow=%s",
				currentCpus, freeCpus, removeFrom, removeSingleFrom)
		}
		removeFrom = removeFrom.Union(removeSingleFrom)
		currentCpus = currentCpus.Difference(removeSingleFrom)
		freeCpus = freeCpus.Union(removeSingleFrom)
	}
	return addFrom, removeFrom, nil
}

func (ta *cpuTreeAllocator) resizeCpus(currentCpus, freeCpus cpuset.CPUSet, delta int) (cpuset.CPUSet, cpuset.CPUSet, error) {
	tnas := ta.root.ToAttributedSlice(currentCpus, freeCpus,
		func(tna *cpuTreeNodeAttributes) bool {
			// filter out branches with insufficient cpus
			if delta > 0 && tna.freeCpuCount-delta < 0 {
				// cannot allocate delta cpus
				return false
			}
			if delta < 0 && tna.currentCpuCount+delta < 0 {
				// cannot release delta cpus
				return false
			}
			return true
		})

	// Sort based on attributes
	// TODO: Parameterize the sort function based on
	// - Spread or pack tightly balloons to resource zones
	if delta > 0 {
		sort.Slice(tnas, ta.sorterAllocate(tnas))
	} else {
		sort.Slice(tnas, ta.sorterRelease(tnas))
	}
	for i, tna := range tnas {
		log.Debugf("%d: %s", i, tna)
	}
	if len(tnas) == 0 {
		return freeCpus, currentCpus, fmt.Errorf("not enough free CPUs")
	}
	return tnas[0].freeCpus, tnas[0].currentCpus, nil
}

type cpuTreeNodeAttributes struct {
	t                *cpuTreeNode
	depth            int
	currentCpus      cpuset.CPUSet
	freeCpus         cpuset.CPUSet
	currentCpuCount  int
	currentCpuCounts []int
	freeCpuCount     int
	freeCpuCounts    []int
}

func (tna cpuTreeNodeAttributes) String() string {
	return fmt.Sprintf("%s{%d,%v,%d,%d}", tna.t.name, tna.depth,
		tna.currentCpuCounts,
		tna.freeCpuCount, tna.freeCpuCounts)
}

func (t *cpuTreeNode) ToAttributedSlice(
	currentCpus, freeCpus cpuset.CPUSet, filter func(*cpuTreeNodeAttributes) bool) []cpuTreeNodeAttributes {
	tnas := []cpuTreeNodeAttributes{}
	currentCpuCounts := []int{}
	freeCpuCounts := []int{}
	t.toAttributedSlice(currentCpus, freeCpus, filter, &tnas, 0, currentCpuCounts, freeCpuCounts)
	return tnas
}

func (t *cpuTreeNode) toAttributedSlice(
	currentCpus, freeCpus cpuset.CPUSet, filter func(*cpuTreeNodeAttributes) bool,
	tnas *[]cpuTreeNodeAttributes, depth int, currentCpuCounts []int, freeCpuCounts []int) {
	currentCpusHere := t.cpus.Intersection(currentCpus)
	freeCpusHere := t.cpus.Intersection(freeCpus)
	currentCpuCountHere := currentCpusHere.Size()
	currentCpuCountsHere := make([]int, len(currentCpuCounts)+1, len(currentCpuCounts)+1)
	copy(currentCpuCountsHere, currentCpuCounts)
	currentCpuCountsHere[depth] = currentCpuCountHere

	freeCpuCountHere := freeCpusHere.Size()
	freeCpuCountsHere := make([]int, len(freeCpuCounts)+1, len(freeCpuCounts)+1)
	copy(freeCpuCountsHere, freeCpuCounts)
	freeCpuCountsHere[depth] = freeCpuCountHere

	tna := cpuTreeNodeAttributes{
		t:                t,
		depth:            depth,
		currentCpus:      currentCpusHere,
		freeCpus:         freeCpusHere,
		currentCpuCount:  currentCpuCountHere,
		currentCpuCounts: currentCpuCountsHere,
		freeCpuCount:     freeCpuCountHere,
		freeCpuCounts:    freeCpuCountsHere,
	}

	if filter != nil && !filter(&tna) {
		return
	}

	*tnas = append(*tnas, tna)
	for _, child := range t.children {
		child.toAttributedSlice(currentCpus, freeCpus, filter,
			tnas, depth+1, currentCpuCountsHere, freeCpuCountsHere)
	}
}

var cpuTopologyLevelToString = map[CPUTopologyLevel]string{
	CPUTopologyLevelUndefined: "undefined",
	CPUTopologyLevelSystem:    "system",
	CPUTopologyLevelPackage:   "package",
	CPUTopologyLevelDie:       "die",
	CPUTopologyLevelNuma:      "numa",
	CPUTopologyLevelCore:      "core",
	CPUTopologyLevelThread:    "thread",
}

func (ctl CPUTopologyLevel) String() string {
	s, ok := cpuTopologyLevelToString[ctl]
	if ok {
		return s
	}
	return fmt.Sprintf("CPUTopologyLevelUnknown(%d)", ctl)
}

// UnmarshalJSON unmarshals a JSON string to Limit
func (ctl *CPUTopologyLevel) UnmarshalJSON(b []byte) error {
	i, err := strconv.Atoi(string(b))
	if err == nil {
		*ctl = CPUTopologyLevel(i)
		return nil
	}
	name := strings.ToLower(string(b))
	if len(name) > 2 && strings.HasPrefix(name, "\"") && strings.HasSuffix(name, "\"") {
		name = name[1 : len(name)-1]
		for ctlInt, ctlName := range cpuTopologyLevelToString {
			if name == ctlName {
				*ctl = ctlInt
				return nil
			}
		}
	}
	return fmt.Errorf("unknown CPU topology level %q", b)
}
