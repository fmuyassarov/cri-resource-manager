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
	"fmt"
	"strings"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type cpuInTopology struct {
	packageID, dieID, numaID, coreID, threadID, cpuID             int
	packageName, dieName, numaName, coreName, threadName, cpuName string
}

type cpusInTopology map[int]cpuInTopology

func newCpuTreeFromInt5(pdnct [5]int) (*cpuTreeNode, cpusInTopology) {
	pkgs := pdnct[0]
	dies := pdnct[1]
	numas := pdnct[2]
	cores := pdnct[3]
	threads := pdnct[4]
	cpuID := 0
	sysTree := NewCpuTree("system")
	csit := cpusInTopology{}
	for packageID := 0; packageID < pkgs; packageID++ {
		packageTree := NewCpuTree(fmt.Sprintf("p%d", packageID))
		sysTree.AddChild(packageTree)
		for dieID := 0; dieID < dies; dieID++ {
			dieTree := NewCpuTree(fmt.Sprintf("p%dd%d", packageID, dieID))
			packageTree.AddChild(dieTree)
			for numaID := 0; numaID < numas; numaID++ {
				numaTree := NewCpuTree(fmt.Sprintf("p%dd%dn%d", packageID, dieID, numaID))
				dieTree.AddChild(numaTree)
				for coreID := 0; coreID < cores; coreID++ {
					coreTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dc%d", packageID, dieID, numaID, coreID))
					numaTree.AddChild(coreTree)
					for threadID := 0; threadID < threads; threadID++ {
						threadTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dc%dt%d", packageID, dieID, numaID, coreID, threadID))
						coreTree.AddChild(threadTree)
						threadTree.AddCpus(cpuset.NewCPUSet(cpuID))
						csit[cpuID] = cpuInTopology{
							packageID, dieID, numaID, coreID, threadID, cpuID,
							packageTree.name, dieTree.name, numaTree.name, coreTree.name, threadTree.name,
							fmt.Sprintf("cpu%d", cpuID),
						}
						cpuID += 1
					}
				}
			}
		}
	}
	return sysTree, csit
}

func verifyNotOn(t *testing.T, nameContents string, cpus cpuset.CPUSet, csit cpusInTopology) {
	for _, cpuID := range cpus.ToSlice() {
		name := csit[cpuID].threadName
		if strings.Contains(name, nameContents) {
			t.Errorf("cpu%d (%s) in unexpected region %s", cpuID, name, nameContents)
		}
	}
}

func verifySame(t *testing.T, topoLevel string, cpus cpuset.CPUSet, csit cpusInTopology) {
	t.Logf("Verify that cpus %s are on the same %s.", cpus, topoLevel)
	seenName := ""
	seenCpuID := -1
	for _, cpuID := range cpus.ToSlice() {
		cit := csit[cpuID]
		thisName := ""
		thisCpuID := cit.cpuID
		switch topoLevel {
		case "core":
			thisName = cit.coreName
		case "numa":
			thisName = cit.numaName
		case "die":
			thisName = cit.dieName
		case "package":
			thisName = cit.packageName
		}
		if thisName == "" {
			t.Errorf("unexpected (invalid) topology level %q", topoLevel)
		}
		if seenName == "" {
			seenName = thisName
			seenCpuID = cit.cpuID
		}
		if seenName != thisName {
			t.Errorf("cpu on unexpected (not same) %s: cpu%d is in %s, cpu%d is in %s",
				topoLevel,
				seenCpuID, seenName,
				thisCpuID, thisName)
		}
	}
}

/*
topology: [5]int{2, 2, 2, 2, 2},
allocations: []int{
	0,  // cpu on p0/d0/n0/c0/t0
	1,  // cpu on p0/d0/n0/c0/t1
	2,  // cpu on p0/d0/n0/c1/t0
	3,  // cpu on p0/d0/n0/c1/t1
	4,  // cpu on p0/d0/n1/c0/t0
	5,  // cpu on p0/d0/n1/c0/t1
	6,  // cpu on p0/d0/n1/c1/t0
	7,  // cpu on p0/d0/n1/c1/t1
	8,  // cpu on p0/d1/n0/c0/t0
	9,  // cpu on p0/d1/n0/c0/t1
	10, // cpu on p0/d1/n0/c1/t0
	11, // cpu on p0/d1/n0/c1/t1
	12, // cpu on p0/d1/n1/c0/t0
	13, // cpu on p0/d1/n1/c0/t1
	14, // cpu on p0/d1/n1/c1/t0
	15, // cpu on p0/d1/n1/c1/t1
	16, // cpu on p1/d0/n0/c0/t0
	17, // cpu on p1/d0/n0/c0/t1
	18, // cpu on p1/d0/n0/c1/t0
	19, // cpu on p1/d0/n0/c1/t1
	20, // cpu on p1/d0/n1/c0/t0
	21, // cpu on p1/d0/n1/c0/t1
	22, // cpu on p1/d0/n1/c1/t0
	23, // cpu on p1/d0/n1/c1/t1
	24, // cpu on p1/d1/n0/c0/t0
	25, // cpu on p1/d1/n0/c0/t1
	26, // cpu on p1/d1/n0/c1/t0
	27, // cpu on p1/d1/n0/c1/t1
	28, // cpu on p1/d1/n1/c0/t0
	29, // cpu on p1/d1/n1/c0/t1
	30, // cpu on p1/d1/n1/c1/t0
	31, // cpu on p1/d1/n1/c1/t1
},
*/

func TestResizeCpus(t *testing.T) {
	tcases := []struct {
		name                string
		topology            [5]int // package, die, numa, core, thread count
		allocations         []int
		deltas              []int
		allocate            bool
		addToCurrent        []int
		expectCurrentOnSame []string
		expectCurrentNotOn  []string
		expectedAddSizes    []int
		expectedErrors      []string
	}{
		{
			name:             "first allocations",
			topology:         [5]int{2, 2, 2, 2, 2},
			deltas:           []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32},
			expectedAddSizes: []int{0, 1, 2, 4, 4, 8, 8, 8, 16, 16, 16, 32, 32, 32},
		},
		{
			name:           "too large an allocation",
			topology:       [5]int{2, 2, 2, 2, 2},
			deltas:         []int{33},
			expectedErrors: []string{"not enough free CPUs"},
		},
		{
			name:     "inflate",
			topology: [5]int{2, 2, 2, 2, 2},
			allocate: true,
			deltas: []int{
				1, 1, 1, 1, // cpu0..cpu3 on numaN0, dieD0
				1, 3, // cpu4..cpu7 on numaN1, still dieD0
				6, 1, 1, // cpu8..15 on dieD1, still packageP0
			},
			addToCurrent: []int{
				1, 1, 1, 1,
				1, 1,
				1, 1, 1},
			expectCurrentOnSame: []string{
				"core", "core", "numa", "numa",
				"die", "die",
				"package", "package", "package"},
			expectedAddSizes: []int{
				1, 1, 1, 1,
				1, 3,
				8, 1, 1},
		},
		{
			name:     "defragmenting single removals",
			topology: [5]int{2, 2, 2, 2, 2},
			allocations: []int{
				0,  // cpu on p0/d0/n0/c0/t0
				2,  // cpu on p0/d0/n0/c1/t0
				3,  // cpu on p0/d0/n0/c1/t1
				7,  // cpu on p0/d0/n1/c1/t1
				10, // cpu on p0/d1/n0/c1/t0
				17, // cpu on p1/d0/n0/c0/t1
				18, // cpu on p1/d0/n0/c1/t0
			},
			allocate: true,
			deltas: []int{
				-1, // release cpu17 or cpu18
				-1, // release cpu17 or cpu18 => all on same package
				-1, // release cpu10 => all on same die
				-1, // release cpu7 => all on same numa
				-1, // release cpu0 => all on same core
				-1, // release cpu2 or cpu3
				-1, // release cpu2 or cpu3
			},
			expectCurrentOnSame: []string{
				"",
				"package",
				"die",
				"numa",
				"core",
				"core",
				"core",
			},
			expectCurrentNotOn: []string{
				"",
				"p1",
				"p0d1",
				"p0d0n1",
				"p0d0n0c0",
			},
		},
		{
			name:     "defragmenting multi-removals",
			topology: [5]int{2, 2, 2, 2, 2},
			allocations: []int{
				0,  // cpu on p0/d0/n0/c0/t0
				2,  // cpu on p0/d0/n0/c1/t0
				4,  // cpu on p0/d0/n1/c0/t0
				6,  // cpu on p0/d0/n1/c1/t0
				8,  // cpu on p0/d1/n0/c0/t0
				9,  // cpu on p0/d1/n0/c0/t1
				10, // cpu on p0/d1/n0/c1/t0

				24, // cpu on p1/d1/n0/c0/t0
				25, // cpu on p1/d1/n0/c0/t1
				26, // cpu on p1/d1/n0/c1/t0
				27, // cpu on p1/d1/n0/c1/t1
				28, // cpu on p1/d1/n1/c0/t0
				29, // cpu on p1/d1/n1/c0/t1
				30, // cpu on p1/d1/n1/c1/t0
				31, // cpu on p1/d1/n1/c1/t1
			},
			allocate: true,
			deltas: []int{
				-2, // release from p0d1n0
				-1, // release completely p0d1
				-5, // release completely p0, one from p1d1nX
				-3, // release completely p1d1nX => all on same numa
			},
			expectCurrentOnSame: []string{
				"",
				"",
				"die",
				"numa",
			},
			expectCurrentNotOn: []string{
				"",
				"p0d1",
				"p0",
				"",
			},
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			tree, csit := newCpuTreeFromInt5(tc.topology)
			currentCpus := cpuset.NewCPUSet()
			freeCpus := tree.Cpus()
			for _, cpuID := range tc.allocations {
				currentCpus = currentCpus.Union(cpuset.NewCPUSet(cpuID))
				freeCpus = freeCpus.Difference(cpuset.NewCPUSet(cpuID))
			}
			for i, delta := range tc.deltas {
				t.Logf("ResizeCpus(current=%s; free=%s; delta=%d)", currentCpus, freeCpus, delta)
				addFrom, removeFrom, err := tree.ResizeCpus(currentCpus, freeCpus, delta)
				t.Logf("== addFrom=%s; removeFrom=%s, err=%v", addFrom, removeFrom, err)
				if i < len(tc.expectedAddSizes) {
					if tc.expectedAddSizes[i] != addFrom.Size() {
						t.Errorf("expected add size: %d, got %d", tc.expectedAddSizes[i], addFrom.Size())
					}
				}
				if i < len(tc.expectedErrors) {
					if tc.expectedErrors[i] == "" && err != nil {
						t.Errorf("expected nil error, but got %v", err)
					}
					if tc.expectedErrors[i] != "" {
						if err == nil {
							t.Errorf("expected error containing %q, got nil", tc.expectedErrors[i])
						} else if !strings.Contains(fmt.Sprintf("%s", err), tc.expectedErrors[i]) {
							t.Errorf("expected error containing %q, got %q", tc.expectedErrors[i], err)
						}
					}
				}
				if tc.allocate {
					for n, cpuID := range addFrom.ToSlice() {
						if n >= delta {
							break
						}
						freeCpus = freeCpus.Difference(cpuset.NewCPUSet(cpuID))
						currentCpus = currentCpus.Union(cpuset.NewCPUSet(cpuID))

					}
					for n, cpuID := range removeFrom.ToSlice() {
						if n >= -delta {
							break
						}
						freeCpus = freeCpus.Union(cpuset.NewCPUSet(cpuID))
						currentCpus = currentCpus.Difference(cpuset.NewCPUSet(cpuID))
					}
					t.Logf("=> current=%s; free=%s", currentCpus, freeCpus)
					if i < len(tc.expectCurrentOnSame) && tc.expectCurrentOnSame[i] != "" {
						verifySame(t, tc.expectCurrentOnSame[i], currentCpus, csit)
					}
					if i < len(tc.expectCurrentNotOn) && tc.expectCurrentNotOn[i] != "" {
						verifyNotOn(t, tc.expectCurrentNotOn[i], currentCpus, csit)
					}
				}
			}
		})
	}
}
