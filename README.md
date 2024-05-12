# RTP-Server Demo
## Setup
This Demo utilizes Asterisk with ARI redirection of audio using [External Media](https://docs.asterisk.org/Development/Reference-Information/Asterisk-Framework-and-API-Examples/External-Media-and-ARI/). Setup an Asterisk server that uses the Stasis function on extension.conf file and also setup a server using an [ARI library](https://docs.asterisk.org/Configuration/Interfaces/Asterisk-REST-Interface-ARI/ARI-Libraries/?h=ari) to make HTTP requests to redirect audio towards the Demo program.\
With the Demo running make a Sip call to the extension and the Demo should capture all microphone audio and the audio file should be played on the other side (Warning, the sound might be really loud).
