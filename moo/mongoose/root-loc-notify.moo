"Notify descendants, yielding after real hooks and when the scan needs budget.";
{what, where, ?level = 0} = args;
for c in (this.contents)
  if (valid(c))
    handler = respond_to(c, verb);
    if (handler && ((handler[1] != $root_class) || c.contents))
      c:(verb)(what, where, level + 1);
      suspend(0);
    endif
  endif
  yin();
endfor
