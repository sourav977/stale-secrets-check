/*
Copyright 2024 Sourav Patnaik.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"os"

	"k8s.io/client-go/tools/clientcmd"
)

func GetClusterName() string {
	// Get the kubeconfig file.
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		kubeconfigPath = home + "/.kube/config"
	}

	// Build the config from the kubeconfigPath
	_, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "cluster"
	}

	// Load the kubeconfig file to get the current context.
	kubeconfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		//panic(err)
		return "cluster"
	}

	// Get the current context name from the config.
	currentContextName := kubeconfig.CurrentContext
	// Get the cluster name using the current context name.
	currentContext, ok := kubeconfig.Contexts[currentContextName]
	if !ok {
		//panic("failed to get current context")
		return "cluster"
	}
	//fmt.Println("Current Cluster Name:", currentContext.Cluster)
	return currentContext.Cluster
}
