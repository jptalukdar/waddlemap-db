user gives a custom struct to the database. This struct is used for values in the database. 
The same struct needs to be converted to protobuf struct for network communication by the client.

There is a core go module, that implements the database. It has socket which handles the connections and sends it to a transaction manager via channels.
Transaction manager handles the transactions and sends it to the storage manager via channels.
Storage manager handles the actual storage of the data.

The program runs a init module on the startup that checks if the database is initialized or not. If not, it creates a directory waddlemap_db to hold the data. It also creates a config file to store the configuration of the database.

The init module initializes all modules and starts the server. It starts separate threads for all the managers. Each of them communicates via go channels. 

Every transaction is handled via a go-routine. 

Create a python client that connects to the db via socket.
It performs all the operations on the db.