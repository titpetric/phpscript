name: md5, sha1 and hexdec match php
description: >
  The digests return lowercase hex (raw bytes with $binary), a non-string
  argument hashes its string form, and hexdec skips characters outside the
  hex alphabet and overflows past PHP_INT_MAX into a float, so the var_dump
  types match too. Verified against php 8.5.
---
<?php

echo md5("hello"), "\n";
echo md5(""), "\n";
echo md5(123), "\n";
echo strlen(md5("x", true)), "\n";
echo sha1("hello"), "\n";
echo sha1(""), "\n";
echo strlen(sha1("x", true)), "\n";
echo hexdec("ff"), "\n";
echo hexdec("0x1A"), "\n";
echo hexdec(""), "\n";
echo hexdec("zz12zz"), "\n";
echo hexdec("7fffffffffffffff"), "\n";
echo hexdec("8000000000000000"), "\n";
echo hexdec("ffffffffffffffffff"), "\n";
echo hexdec(255), "\n";
var_dump(hexdec("ff"));
var_dump(hexdec("8000000000000000"));
---
5d41402abc4b2a76b9719d911017c592
d41d8cd98f00b204e9800998ecf8427e
202cb962ac59075b964b07152d234b70
16
aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d
da39a3ee5e6b4b0d3255bfef95601890afd80709
20
255
26
0
18
9223372036854775807
9.2233720368548E+18
4.7223664828696E+21
597
int(255)
float(9.223372036854776E+18)
