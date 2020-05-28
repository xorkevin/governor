# Example governor kubernetes deployment

### Create kustomization files

```
./plan.sh
```

### Register service authorization policies

```
./register.sh
```

### Connect governor with services

```
./connect.sh
```

### Create development deployment

```
kubectl apply -k base
```
