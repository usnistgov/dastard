# Check that you have your go path in the bash PATH.
# Use this by sourcing the current file:
# source update-path.sh

dir=`go env GOPATH`/bin
BASHRC=$HOME/.bashrc

if [ `echo :$PATH: | grep -F :$dir:` ]; then
   echo "$dir is already in the UNIX path"
else
   echo "$dir is not in the UNIX path. Updating \$PATH and $BASHRC"
   echo >> $BASHRC
   echo "# Updated by Dastard package update-path.sh" >> $BASHRC
   echo "export PATH=\$PATH:$dir" >> $BASHRC
   export PATH=$PATH:$dir
   echo $PATH
fi

unset dir
unset BASHRC
