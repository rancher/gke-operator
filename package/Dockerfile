FROM registry.suse.com/suse/sle15:15.3
RUN zypper update -y && \
    zypper -n clean -a && \
    rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/*
RUN useradd --uid 1007 gke-operator
ENV KUBECONFIG /home/gke-operator/.kube/config
COPY bin/gke-operator /usr/bin/
USER 1007
ENTRYPOINT ["gke-operator"]
