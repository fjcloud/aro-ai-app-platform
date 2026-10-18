FROM registry.access.redhat.com/ubi10/ubi:latest

RUN dnf -y install tar git gzip findutils which python3 make curl bash \
    && rpm --import https://packages.microsoft.com/keys/microsoft.asc \
    && dnf -y install https://packages.microsoft.com/config/rhel/10/packages-microsoft-prod.rpm \
    && dnf -y install azure-cli \
    && dnf -y clean all \
    && rm -rf /var/cache/dnf \
    && mkdir -p /home/az \
    && printf '%s\n' \
        'export HOME=/home/az' \
        '[ -f /etc/bashrc ] && . /etc/bashrc' \
        'cd /home/az' \
        > /etc/workshop.bashrc

ENV HOME=/home/az \
    AZURE_EXTENSION_DIR=/opt/az/extensions
WORKDIR /home/az
# The published "aro" extension hides --operators-authentication. This wheel is
# the aro-hcp CLI the workshop and aro-hcp-quickstart pass to cluster create.
COPY aro_hcp-1.0.0b3-py3-none-any.whl /tmp/aro_hcp-1.0.0b3-py3-none-any.whl
RUN mkdir -p /opt/az/extensions \
    && az extension add --source /tmp/aro_hcp-1.0.0b3-py3-none-any.whl --yes \
    && rm -f /tmp/aro_hcp-1.0.0b3-py3-none-any.whl \
    && az aro hcp cluster create -h > /tmp/hcp-help.txt \
    && grep -q -- '--operators-authentication' /tmp/hcp-help.txt \
    && grep -q -- '--user-assigned-identities' /tmp/hcp-help.txt \
    && rm -f /tmp/hcp-help.txt \
    && printf '%s\n' "export PS1='az# '" >> /etc/workshop.bashrc
COPY tmux.conf /etc/tmux.conf
COPY workshop-init workshop-shell workshop-bash /usr/local/bin/
RUN dnf -y install openssl \
    && dnf -y install \
        https://dl.rockylinux.org/pub/rocky/10/BaseOS/x86_64/os/Packages/l/libevent-2.1.13-1.el10_2.x86_64.rpm \
        https://dl.rockylinux.org/pub/rocky/10/BaseOS/x86_64/os/Packages/t/tmux-3.3a-13.20250207gitb202a2f.el10.x86_64.rpm \
    && dnf -y clean all \
    && rm -rf /var/cache/dnf \
    && tmux -V \
    && openssl version \
    && chmod 755 /usr/local/bin/workshop-init /usr/local/bin/workshop-shell /usr/local/bin/workshop-bash \
    && mkdir -p /var/tmux \
    && chmod 700 /var/tmux
CMD ["/usr/local/bin/workshop-init"]
