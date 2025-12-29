cd /tmp
kubectl get configmap fluentd-config -n fluent-system -ojsonpath='{.data.04_outputs\.conf}' > outputs.conf
CONFIG_DATA=$(cat outputs.conf)
NEW_CONFIG_DATA="$CONFIG_DATA"
 
if echo $NEW_CONFIG_DATA | grep -q "#{ENV\['LOGINSIGHT_HOST'\]}"; then
    NEW_CONFIG_DATA=$(printf "%s" "$NEW_CONFIG_DATA" | sed "s/#{ENV\['LOGINSIGHT_HOST'\]}//" )
else
    echo host "#{ENV\['LOGINSIGHT_HOST'\]}" not found
fi
 
if echo $NEW_CONFIG_DATA | grep -q "#{ENV\['LOGINSIGHT_PORT'\]}"; then
    NEW_CONFIG_DATA=$(printf "%s" "$NEW_CONFIG_DATA" | sed "s/#{ENV\['LOGINSIGHT_PORT'\]}//" )
else
    echo host "#{ENV\['LOGINSIGHT_PORT'\]}" not found
fi
 
if diff <(echo "$CONFIG_DATA") <(echo "$NEW_CONFIG_DATA") >/dev/null; then
    echo "Change not detected"
else
    echo "Change detected. Updating configmap..."
    echo "------"
    echo Old configmap data:
    echo "$CONFIG_DATA"
    echo "------"
    echo New configmap data:
    echo "$NEW_CONFIG_DATA"
    echo "$NEW_CONFIG_DATA" > 04_outputs.conf
    echo "------"
 
    kubectl create configmap fluentd-config -n fluent-system --from-file=04_outputs.conf --dry-run=client -oyaml | \
    kubectl patch configmap fluentd-config -n fluent-system --type merge --patch-file /dev/stdin
 
fi