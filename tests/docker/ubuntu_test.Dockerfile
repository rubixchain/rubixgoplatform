FROM amd64/ubuntu

# Install python3 and requests dependency 
RUN apt-get update && apt-get install -y python3 python3-requests wget build-essential tmux

# Install golang 1.21
RUN wget https://go.dev/dl/go1.21.10.linux-amd64.tar.gz
RUN tar -C /usr/local -xzf go1.21.10.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

# Copy the tests directory
COPY . /usr/node/

# Change directory to test
WORKDIR /usr/node/tests

# Run tests
CMD ["python3", "-u", "run.py"]
