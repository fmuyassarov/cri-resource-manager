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
	"sort"
	"strings"
	"testing"

	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

type cpuInTopology struct {
	packageID, dieID, numaID, coreID, threadID, cpuID             int
	packageName, dieName, numaName, coreName, threadName, cpuName string
}

type cpusInTopology map[int]cpuInTopology

func (cit cpuInTopology) TopoName(topoLevel string) string {
	switch topoLevel {
	case "thread":
		return cit.threadName
	case "core":
		return cit.coreName
	case "numa":
		return cit.numaName
	case "die":
		return cit.dieName
	case "package":
		return cit.packageName
	}
	panic("invalid topoLevel")
}

func (csit cpusInTopology) dumps(nameCpus map[string]cpuset.CPUSet) string {
	lines := []string{}
	names := make([]string, 0, len(nameCpus))
	for name := range nameCpus {
		names = append(names, name)
	}
	sort.Strings(names)
	for cpuID := 0; cpuID < len(csit); cpuID++ {
		line := fmt.Sprintf("cpu%02d %s", cpuID, csit[cpuID].threadName)
		for _, name := range names {
			if nameCpus[name].Contains(cpuID) {
				line = fmt.Sprintf("%s %s", line, name)
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

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
					coreTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dc%02d", packageID, dieID, numaID, coreID))
					numaTree.AddChild(coreTree)
					for threadID := 0; threadID < threads; threadID++ {
						threadTree := NewCpuTree(fmt.Sprintf("p%dd%dn%dc%02dt%d", packageID, dieID, numaID, coreID, threadID))
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
	seenName := ""
	seenCpuID := -1
	for _, cpuID := range cpus.ToSlice() {
		cit := csit[cpuID]
		thisName := cit.TopoName(topoLevel)
		thisCpuID := cit.cpuID
		if thisName == "" {
			t.Errorf("unexpected (invalid) topology level %q", topoLevel)
		}
		if seenName == "" {
			seenName = thisName
			seenCpuID = cit.cpuID
		}
		if seenName != thisName {
			t.Errorf("expected the same %s, got: cpu%d in %s, cpu%d in %s",
				topoLevel,
				seenCpuID, seenName,
				thisCpuID, thisName)
		}
	}
}

func (csit cpusInTopology) getElements(topoLevel string, cpus cpuset.CPUSet) []string {
	elts := []string{}
	for _, cpuID := range cpus.ToSlice() {
		elts = append(elts, csit[cpuID].TopoName(topoLevel))
	}
	return elts
}

func (csit cpusInTopology) verifyDisjoint(t *testing.T, topoLevel string, cpusA cpuset.CPUSet, cpusB cpuset.CPUSet) {
	eltsA := csit.getElements(topoLevel, cpusA)
	eltsB := csit.getElements(topoLevel, cpusB)
	for _, eltA := range eltsA {
		for _, eltB := range eltsB {
			if eltA == eltB {
				t.Errorf("expected disjoint %ss, got %s on both cpusets %s and %s",
					topoLevel, eltA, cpusA, cpusB)
				return
			}
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
	type TopoCcids struct {
		topo  string
		ccids []int
	}
	tcases := []struct {
		name                string
		topology            [5]int // package, die, numa, core, thread count
		allocatorTB         bool   // allocator topologyBalancing
		allocations         []int
		deltas              []int
		allocate            bool
		operateOnCcid       []int // which ccid (currentCpus id) will be used on call
		expectCurrentOnSame []string
		expectAllOnSame     []string
		expectCurrentNotOn  []string
		expectAddSizes      []int
		expectDisjoint      []TopoCcids // which ccids should be disjoint
		expectErrors        []string
	}{
		{
			name:           "first allocations",
			topology:       [5]int{2, 2, 2, 2, 2},
			deltas:         []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32},
			expectAddSizes: []int{0, 1, 2, 4, 4, 8, 8, 8, 16, 16, 16, 32, 32, 32},
		},
		{
			name:         "too large an allocation",
			topology:     [5]int{2, 2, 2, 2, 2},
			deltas:       []int{33},
			expectErrors: []string{"not enough free CPUs"},
		},
		{
			name:          "spread allocations",
			topology:      [5]int{2, 2, 2, 2, 2},
			allocatorTB:   true,
			deltas:        []int{1, 1, 1, 1, 1, 1, 1, 1},
			allocate:      true,
			operateOnCcid: []int{1, 2, 3, 4, 5, 6, 7, 8},
			expectDisjoint: []TopoCcids{
				{},
				{"package", []int{1, 2}},
				{"die", []int{1, 2, 3}},
				{"die", []int{1, 2, 3, 4}},
				{"numa", []int{1, 2, 3, 4, 5}},
				{"numa", []int{1, 2, 3, 4, 5, 6}},
				{"numa", []int{1, 2, 3, 4, 5, 6, 7}},
				{"numa", []int{1, 2, 3, 4, 5, 6, 7, 8}},
			},
		},
		{
			name:          "pack allocations",
			topology:      [5]int{2, 2, 2, 2, 2},
			allocatorTB:   false,
			deltas:        []int{1, 1, 1, 1},
			allocate:      true,
			operateOnCcid: []int{1, 2, 3, 4, 5},
			expectAllOnSame: []string{
				"", "core", "numa", "numa", "die", "die",
			},
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
			operateOnCcid: []int{
				1, 1, 1, 1,
				1, 1,
				1, 1, 1},
			expectCurrentOnSame: []string{
				"core", "core", "numa", "numa",
				"die", "die",
				"package", "package", "package"},
			expectAddSizes: []int{
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
			operateOnCcid: []int{1, 1, 1, 1, 1, 1, 1},
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
				"p0d0n0c00",
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
			operateOnCcid: []int{1, 1, 1, 1},
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
		{
			name:     "gentle rebalancing",
			topology: [5]int{2, 1, 1, 16, 2}, // 2 packages, 16 hyperthreaded cores per package => 64 cpus in total
			deltas: []int{
				4, 4, 14, 7, 7, 4, 4, 14, // allocate 8 sets of cpus, the last 14cpus fills package0, spills over to package1
				-2, -2, -2, -2, // free a little room to package0
				-1, 1, -1, 1, -1, 1, -1, 1}, // deflate/inflate the last 14cpus, see that it gradually travels to package0
			operateOnCcid: []int{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4,
				8, 8, 8, 8, 8, 8, 8, 8,
			},
			allocate: true,
			expectCurrentOnSame: []string{
				"package", "package", "package", "package",
				"package", "package", "package", "",
				"", "", "", "",
				"", "", "", "", "", "", "package", "package",
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
			treeA := tree.NewAllocator(cpuTreeAllocatorOptions{
				topologyBalancing: tc.allocatorTB,
			})
			currentCpus := cpuset.NewCPUSet()
			freeCpus := tree.Cpus()
			if len(tc.allocations) > 0 {
				currentCpus = currentCpus.Union(cpuset.NewCPUSet(tc.allocations...))
				freeCpus = freeCpus.Difference(cpuset.NewCPUSet(tc.allocations...))
			}
			ccidCurrentCpus := map[int]cpuset.CPUSet{0: currentCpus}
			allocs := map[string]cpuset.CPUSet{"--:allo": currentCpus}
			for i, delta := range tc.deltas {
				if i < len(tc.operateOnCcid) && tc.operateOnCcid[i] > 0 {
					currentCpus = ccidCurrentCpus[tc.operateOnCcid[i]]
				}
				t.Logf("ResizeCpus(current=%s; free=%s; delta=%d)", currentCpus, freeCpus, delta)
				addFrom, removeFrom, err := treeA.ResizeCpus(currentCpus, freeCpus, delta)
				t.Logf("== addFrom=%s; removeFrom=%s, err=%v", addFrom, removeFrom, err)
				if i < len(tc.expectAddSizes) {
					if tc.expectAddSizes[i] != addFrom.Size() {
						t.Errorf("expected add size: %d, got %d", tc.expectAddSizes[i], addFrom.Size())
					}
				}
				if i < len(tc.expectErrors) {
					if tc.expectErrors[i] == "" && err != nil {
						t.Errorf("expected nil error, but got %v", err)
					}
					if tc.expectErrors[i] != "" {
						if err == nil {
							t.Errorf("expected error containing %q, got nil", tc.expectErrors[i])
						} else if !strings.Contains(fmt.Sprintf("%s", err), tc.expectErrors[i]) {
							t.Errorf("expected error containing %q, got %q", tc.expectErrors[i], err)
						}
					}
				}
				if tc.allocate {
					allocName := fmt.Sprintf("%02d:allo", i+1)
					allocs[allocName] = cpuset.NewCPUSet()

					for n, cpuID := range addFrom.ToSlice() {
						if n >= delta {
							break
						}
						freeCpus = freeCpus.Difference(cpuset.NewCPUSet(cpuID))
						currentCpus = currentCpus.Union(cpuset.NewCPUSet(cpuID))
						allocs[allocName] = allocs[allocName].Union(cpuset.NewCPUSet(cpuID))
					}
					allocName = fmt.Sprintf("%02d:free", i+1)
					for n, cpuID := range removeFrom.ToSlice() {
						if n >= -delta {
							break
						}
						freeCpus = freeCpus.Union(cpuset.NewCPUSet(cpuID))
						if i < len(tc.operateOnCcid) && tc.operateOnCcid[i] > 0 {
							currentCpus = currentCpus.Difference(cpuset.NewCPUSet(cpuID))
						}
						allocs[allocName] = allocs[allocName].Union(cpuset.NewCPUSet(cpuID))
					}
					if i < len(tc.operateOnCcid) && tc.operateOnCcid[i] > 0 {
						ccidCurrentCpus[tc.operateOnCcid[i]] = currentCpus
					}

					allocs["free"] = freeCpus
					t.Logf("=> current=%s; free=%s", currentCpus, freeCpus)
					if i < len(tc.expectCurrentOnSame) && tc.expectCurrentOnSame[i] != "" {
						verifySame(t, tc.expectCurrentOnSame[i], currentCpus, csit)
					}
					if i < len(tc.expectCurrentNotOn) && tc.expectCurrentNotOn[i] != "" {
						verifyNotOn(t, tc.expectCurrentNotOn[i], currentCpus, csit)
					}
					if i < len(tc.expectAllOnSame) && tc.expectAllOnSame[i] != "" {
						allCpus := cpuset.NewCPUSet()
						for _, cpus := range ccidCurrentCpus {
							allCpus = allCpus.Union(cpus)
						}
						verifySame(t, tc.expectAllOnSame[i], allCpus, csit)
					}

					if i < len(tc.expectDisjoint) && len(tc.expectDisjoint) > 1 {
						for first := 0; first < len(tc.expectDisjoint[i].ccids); first++ {
							for second := first + 1; second < len(tc.expectDisjoint[i].ccids); second++ {
								csit.verifyDisjoint(t, tc.expectDisjoint[i].topo,
									ccidCurrentCpus[tc.expectDisjoint[i].ccids[first]],
									ccidCurrentCpus[tc.expectDisjoint[i].ccids[second]])
							}
						}
					}
				}
				if t.Failed() {
					t.Logf("current and free cpus:\n%s\n", csit.dumps(allocs))
					break
				}
			}
		})
	}
}
