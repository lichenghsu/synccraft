# SSH bastion for tunnel integration tests. Test-only credentials.
FROM docker.io/library/alpine:3.22
RUN apk add --no-cache openssh-server \
 && ssh-keygen -A \
 && adduser -D -s /bin/sh ops \
 && echo 'ops:ops-pass' | chpasswd \
 && sed -i -E \
      -e 's/^#?AllowTcpForwarding .*/AllowTcpForwarding yes/' \
      -e 's/^#?PasswordAuthentication .*/PasswordAuthentication yes/' \
      /etc/ssh/sshd_config \
 && grep -q '^AllowTcpForwarding yes' /etc/ssh/sshd_config
EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
