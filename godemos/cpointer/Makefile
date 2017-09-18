%.o: %.c
	$(CC) $(CFLAGS) -c -o $@ $<

libcptr.a: cpointer.o
	ar -rcs $@ $<
