package main

import (
	dag "github.com/seoyhaein/dag-go"
)

func main() {
	// TODO 버그가 있지만 당분간은 버그 수정 않할 예정임.
	xml := `
	<nodes>
		<node id = "1">
			<to>2</to>
			<command> echo "hello world 1"</command>
		</node>
		<node id = "2" >
			<from>1</from>
			<to>3</to>
			<to>8</to>
			<command>echo "hello world 2"</command>
		</node>
		<node id ="3" >
			<from>2</from>
			<to>4</to>
			<to>5</to>
			<command>echo "hello world 3"</command>
		</node>
		<node id ="4" >
			<from>3</from>
			<to>6</to>
			<command>echo "hello world 4"</command>
		</node>
		<node id ="5" >
			<from>3</from>
			<to>6</to>
			<command>echo "hello world 5"</command>
		</node>
		<node id ="6" >
			<from>4</from>
			<from>5</from>
			<to>7</to>
			<command>echo "hello world 6"</command>
		</node>
		<node id ="7" >
			<from>6</from>
			<to>9</to>
			<to>10</to>
			<command>echo "hello world 7"</command>
		</node>
		<node id ="8" >
			<from>2</from>
			<command>echo "hello world 8"</command>
		</node>
		<node id ="9" >
			<from>7</from>
			<to>11</to>
			<command>echo "hello world 9"</command>
		</node>
		<node id ="10" >
			<from>7</from>
			<to>11</to>
			<command>echo "hello world 10"</command>
		</node>
		<node id ="11" >
			<from>9</from>
			<from>10</from>
			<command>
					sleep 1
					echo "Hello World 11"
					sleep 1
					echo "one"
					sleep 1
					echo "two"
					sleep 1
					echo "three"
					sleep 1
					echo "four"
					sleep 1
					echo "Sleep 10s"
					sleep 10s
					echo "End"
		</command>
		</node>
	</nodes>`

	d := []byte(xml)
	_, graph := dag.XmlParser(d)

	graph.Start()
}
