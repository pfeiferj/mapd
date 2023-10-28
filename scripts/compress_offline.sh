#!/bin/bash
for line in `find ./offline/*/* -type d`;
do tar -czvf ${line}.tar.gz $line;
done
