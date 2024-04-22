# StaleSecretWatch
StaleSecretWatch is a Kubernetes Operator designed to monitor and identify stale secrets across namespaces within a Kubernetes cluster. A "stale secret" is defined as a secret that has not been updated within a specified threshold (e.g., 90 days), potentially increasing the risk of compromise.

This operator actively scans for such secrets and implements adaptive alerting to notify relevant application management teams, helping to maintain high security and compliance standards.


### Prerequisites
- Golang
- A Kubernetes cluster (e.g., Docker, Lima, Kind, Rancher Desktop, Minikube, or any cloud Kubernetes cluster provider)
- A Container runtime
- Slack channel ID to send notification
- Slack [Bot User OAuth Token](https://api.slack.com/authentication/token-types#bot)
- Check out the [quickstart guide for creating a Slack app](https://api.slack.com/start/quickstart)

Once you have Slack Channel ID and Bot User OAuth Token, update them in `config/manager/manager.yaml`

```
  SLACK_BOT_TOKEN: base64-encoded-token-here
  SLACK_CHANNEL_ID: base64-encoded-channel-id-here
```

## Installation and Running

### Method 1: Local Execution using Make

1. **Install Cert-Manager**

Cert-manager is required to run StaleSecretWatch locally:
```
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
```

Wait for 5-10 minutes until cert-manager is fully operational and the API reports readiness. 
`The cert-manager API is ready`.

2. **Setup Certificates**

Create the necessary directories and populate them with the required certificate files:
In my case when I run locally using `make run` , it complains that certs are not present in below path. So I have created accordingly. In your case, run `make run` first without installing above cert-manager to get the path.

```
mkdir -p /var/folders/1g/rxrzb8ss4kbfgjh5zdd9w_cm0000gn/T/k8s-webhook-server/serving-certs/

kubectl get secret cert-manager-webhook-ca -n cert-manager  -o jsonpath='{.data.ca\.crt}' | base64 -d > /var/folders/1g/rxrzb8ss4kbfgjh5zdd9w_cm0000gn/T/k8s-webhook-server/serving-certs/ca.crt

kubectl get secret cert-manager-webhook-ca -n cert-manager  -o jsonpath='{.data.tls\.crt}' | base64 -d > /var/folders/1g/rxrzb8ss4kbfgjh5zdd9w_cm0000gn/T/k8s-webhook-server/serving-certs/tls.crt

kubectl get secret cert-manager-webhook-ca -n cert-manager  -o jsonpath='{.data.tls\.key}' | base64 -d > /var/folders/1g/rxrzb8ss4kbfgjh5zdd9w_cm0000gn/T/k8s-webhook-server/serving-certs/tls.key
```

3. **Remove Cert-Manager**

```
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
```

4. **Run the Operator Locally**

```
export SLACK_BOT_TOKEN=<slack bot token>
export SLACK_CHANNEL_ID=<slack channel ID>
make
make crds.install
make run
```

This will start the operator and wait for StaleSecretWatch resources to be created. The controller will run and wait for and any `StaleSecretWatch` resource to get create.

5. **Deploy a `StaleSecretWatch` Resource**

In a new terminal, deploy a sample StaleSecretWatch resource. You can find the Sample yaml files in test-yaml directory. Run any yaml.
example:
```
kubectl create -f test-yaml/test-ssw.yaml
```

6. **Helpful Commands**

Monitor the resources and view logs with:

```
kubectl describe ssw stalesecretwatch-sample

kubectl events --for ssw/StaleSecretWatch-sample --watch

kubectl get ssw StaleSecretWatch-sample -o jsonpath="{.status.conditions}" --watch 
```

7. **View the Generated ConfigMap**

Based on the secrets and namespace mentioned in the yaml file, `StaleSecretWatch` resource will create a configmap resource named `hashed-secrets-stalesecretwatch` in `default` namespace which will contain the hash of secret data. To see the data stored by StaleSecretWatch in a ConfigMap:

```
kubectl get cm hashed-secrets-stalesecretwatch -o jsonpath='{.binaryData.data}' | base64 -d | jq .
```

8. **Watch changes**

Now update any secret resource's data section from the watchlist you have provided in yaml file, save it, now check again the `hashed-secrets-stalesecretwatch` data

for example:

```
kubectl edit secret chef-user-secret -n vivid

kubectl get secret chef-user-secret -n vivid -o jsonpath='{@}' | jq .

kubectl get cm hashed-secrets-stalesecretwatch -o jsonpath='{.binaryData.data}' | base64 -d | jq .
```

9. **Slack Notification**

Sample Slack Notifications

<details>
<summary>Warning Message</summary>

![warning_msg](/images/warning_msg.png)

</details>

<br>
<details>
<summary>Normal Message</summary>

![normal_msg](/images/normal_msg.png)

</details>
<br>

10. **Deletion**

To delete a `StaleSecretWatch` resource and its associated configurations:

Run 

```
kubectl delete -f test-yaml/test-ssw.yaml
```

11. **Remove crds**

Run

```
make crds.uninstall
```

### Method 2: Run in a Kubernetes Cluster

I have created a script to run StaleSecretWatch in local or any Kubernetes Cluster. To deploy StaleSecretWatch in a Kubernetes cluster:

Run

```
./run_locally.sh
```

Follow Steps 5 onwards from **Method1**
