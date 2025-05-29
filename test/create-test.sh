#!/bin/bash

set -e

NAMESPACE=default

# List of CRDs to create
CRDS=("myapps" "workflows" "pipelines" "tasks" "experiments")
KINDS=("MyApp" "Workflow" "Pipeline" "Task" "Experiment")
PLURALS=("myapps" "workflows" "pipelines" "tasks" "experiments")

# Create CRDs
for i in "${!CRDS[@]}"; do
  PLURAL=${PLURALS[$i]}
  KIND=${KINDS[$i]}

  kubectl apply -f - <<EOF
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: ${PLURAL}.demo.bastion.io
spec:
  group: demo.bastion.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              x-kubernetes-preserve-unknown-fields: true
  scope: Namespaced
  names:
    plural: ${PLURAL}
    singular: ${PLURAL%?}
    kind: ${KIND}
    shortNames:
      - ${PLURAL:0:2}
EOF

done

# Wait for CRDs to register
sleep 5

# Create CRs for each kind
for i in "${!KINDS[@]}"; do
  KIND=${KINDS[$i]}
  PLURAL=${PLURALS[$i]}
  NAME="sample-${PLURAL}"

  kubectl apply -f - <<EOF
apiVersion: demo.bastion.io/v1
kind: ${KIND}
metadata:
  name: ${NAME}
  namespace: ${NAMESPACE}
  annotations:
    backup.bastion.io/enabled: "true"
spec:
  dummy: "data"
EOF

done

# Wait for backup controller to process
sleep 5

# Update CRs to simulate changes
for i in "${!KINDS[@]}"; do
  KIND=${KINDS[$i]}
  PLURAL=${PLURALS[$i]}
  NAME="sample-${PLURAL}"
  kubectl patch ${PLURAL} ${NAME} -n ${NAMESPACE} --type=merge -p '{"spec":{"dummy":"updated-data"}}'
  sleep 1

done

# Wait again for backup controller to process updates
sleep 5

echo "Backup files should now reflect updated data in the backup directory."

# Cleanup resources
for i in "${!KINDS[@]}"; do
  KIND=${KINDS[$i]}
  PLURAL=${PLURALS[$i]}
  NAME="sample-${PLURAL}"
  kubectl delete ${PLURAL} ${NAME} -n ${NAMESPACE} || true
  sleep 1
  kubectl delete crd ${PLURAL}.demo.bastion.io || true
done
