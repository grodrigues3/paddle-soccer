// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nodescaler

import (
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
	core "k8s.io/client-go/testing"
)

func TestScaleUpNodes(t *testing.T) {
	nodes := &v1.NodeList{Items: []v1.Node{
		{Spec: v1.NodeSpec{Unschedulable: false}, ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
				Conditions: readyNodeCondition}},
		{Spec: v1.NodeSpec{Unschedulable: false}, ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{"app": "game-server"}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("2.0")},
				Conditions: readyNodeCondition}}}}

	assertAllUnscheduled(t, nodes)

	cs := &fake.Clientset{}
	cs.AddReactor("list", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		return true, nodes, nil
	})

	s, err := NewServer("", "app=game-server", "0.5", 5, time.Second)
	assert.Nil(t, err)
	s.cs = cs

	expected := int64(len(nodes.Items))
	mock := &NodePoolMock{size: expected}
	s.nodePool = mock
	err = s.scaleNodes()
	assert.Nil(t, err)
	assert.Equal(t, expected, mock.size)
	assertAllUnscheduled(t, nodes)

	cs.AddReactor("list", "pods", func(a core.Action) (bool, runtime.Object, error) {
		if a.(core.ListAction).GetListRestrictions().Fields.String() == "spec.nodeName=foo" {
			return true,
				&v1.PodList{Items: []v1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.5")}}}}}},
					{ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.3")}}}}}}}}, nil
		}
		return false, nil, nil
	})

	s.nodePool = mock
	err = s.scaleNodes()
	assert.Nil(t, err)
	assert.Equal(t, expected, mock.size)
	assertAllUnscheduled(t, nodes)

	cs.AddReactor("list", "pods", func(a core.Action) (bool, runtime.Object, error) {
		if a.(core.ListAction).GetListRestrictions().Fields.String() == "spec.nodeName=bar" {
			return true,
				&v1.PodList{Items: []v1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1.8")}}}}}}}}, nil
		}
		return false, nil, nil
	})

	s.nodePool = mock
	err = s.scaleNodes()
	assert.Nil(t, err)
	assert.Equal(t, int64(3), mock.size)
	assertAllUnscheduled(t, nodes)
}

// assertAllUnscheduled checks all nodes are unscheduled
func assertAllUnscheduled(t *testing.T, nodes *v1.NodeList) {
	for _, n := range nodes.Items {
		assert.False(t, n.Spec.Unschedulable, "Node %v, should not be schedulable", n.Name)
	}
}

type NodePoolMock struct {
	size int64
}

// IncreaseToSize
func (npm *NodePoolMock) IncreaseToSize(size int64) error {
	if size <= npm.size {
		log.Printf("[Test][Mock:IncreaseToSize] Ignoring resize to %v, as size is already %v", size, npm.size)
		return nil
	}
	log.Printf("[Test][Mock:IncreaseToSize] Resising to: %v", size)
	npm.size = size
	return nil
}

func TestScaleDownCordonNodes(t *testing.T) {
	nodes := &v1.NodeList{Items: []v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{"app": "game-server"}, Annotations: map[string]string{}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("5.0")},
				Conditions: readyNodeCondition}},
		{ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{"app": "game-server"}, Annotations: map[string]string{}},
			Status: v1.NodeStatus{Capacity: v1.ResourceList{v1.ResourceCPU: resource.MustParse("5.0")},
				Conditions: readyNodeCondition}}}}

	cs := &fake.Clientset{}
	cs.AddReactor("list", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		return true, nodes, nil
	})
	cs.AddReactor("update", "nodes", func(a core.Action) (bool, runtime.Object, error) {
		ua := a.(core.UpdateAction)
		n := ua.GetObject().(*v1.Node)

		for i, ns := range nodes.Items {
			if ns.Name == n.Name {
				nodes.Items[i] = *n
			}
		}

		return true, n, nil
	})

	s, err := NewServer("", "app=game-server", "0.5", 5, time.Second)
	assert.Nil(t, err)
	s.cs = cs

	// Make sure at least one of them is unscheduled
	err = s.scaleNodes()
	assert.Nil(t, err)
	u := 0
	nl, err := s.newNodeList()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nl.availableNodes()))

	for _, n := range nl.nodes.Items {
		if n.Spec.Unschedulable {
			u++
		}
	}
	assert.Equal(t, 1, u)

	// reset nodes, and up their capacity to 6.0
	for i, n := range nodes.Items {
		n.Spec.Unschedulable = false
		n.Status.Capacity.Cpu().Set(6)
		nodes.Items[i] = n
	}

	cs.AddReactor("list", "pods", func(a core.Action) (bool, runtime.Object, error) {
		if a.(core.ListAction).GetListRestrictions().Fields.String() == "spec.nodeName=bar" {
			return true,
				&v1.PodList{Items: []v1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("0.5")}}}}}}}}, nil
		}
		return false, nil, nil
	})

	log.Printf("[Test][%v] Scaling down after resetting capacity to 6, and adding a single pod to foo.", t.Name())
	err = s.scaleNodes()
	assert.Nil(t, err)
	nl, err = s.newNodeList()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nl.availableNodes()))
	assert.Equal(t, "foo", nl.nodes.Items[0].Name)
}
