#!/bin/bash

threads=16
counter=0
touch ch
for line in `find offline/*/* -type d`;
do
  if [ $counter -lt $threads ]; then
    { tar -czvf ${line}.tar.gz $line; echo 'done' > ch; } &
    let $[counter++];
  else
    read x < ch # waiting for a process to finish
    { tar -czvf ${line}.tar.gz $line; echo 'done' > ch; } &
  fi
done

wait # wait for remaining background processes

rm -f ch

echo 'finished compressing files'
