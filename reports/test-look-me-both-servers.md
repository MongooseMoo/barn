# Test Results: "look me" on Toast and Barn

## Toast Output (port 7777)

```
*** Connected ***
#$#mcp version: 2.1 to: 2.1
The First Room
This is all there is right now.
ANSI Version 2.6 is currently active.  Type "?ansi-intro" for more information.
Your previous connection was before we started keeping track.
There is new news.  Type `news' to read all news or `news new' to read just new news.
Wizard
You see a wizard who chooses not to reveal its true appearance.
It is awake and looks alert.
```

## Barn Output (port 9300)

```
Welcome to the ToastCore database.

Type 'connect wizard' to log in.

You will probably want to change this text and the output of the `help' command, which are stored in $login.welcome_message and $login.help_message, respectively.

ANSI Version 2.6 is currently active.  Type "?ansi-intro" for more information.
Your previous connection was before we started keeping track.
There is new news.  Type `news' to read all news or `news new' to read just new news.
Wizard
You see a wizard who chooses not to reveal its true appearance.
#2 <- #41:_do (this == #41), line 33:  Verb not found
#2 <- ... called from #41:pronoun_sub (this == #41), line 27
#2 <- ... called from #6:look_self (this == #6), line 6
#2 <- ... called from #3:l*ook (this == #62), line 13
#2 <- (End of traceback)
```

## Difference

Barn produces a "Verb not found" error traceback during `look me`, while Toast completes successfully and shows "It is awake and looks alert."
