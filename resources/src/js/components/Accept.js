import React, { Component } from 'react';
import Dropzone from 'react-dropzone';
import Card from 'grommet/components/Card';
import Heading from 'grommet/components/Heading';

export default class AcceptTarget extends Component {
  constructor() {
    super();
    this.state = {
      accepted: [],
      rejected: [],
      manifest: null
    };
  }

  // When a valid item is dropped, fetch an S3 URL and put that item to
  // the S3 URL. Then setup a polling loop and wait until the data is
  // available.
  onDrop(files) {
    console.log(JSON.stringify(files, '' ,' '));
    var headFile = files.pop();
    if (headFile && this.props.submitFileHandler) {
      this.props.submitFileHandler(headFile);
      this.setState({
        preview_data: headFile.preview
      });
    }
  }

  render() {
    return (
      <Card
        contentPad="large"
        heading={
          <Heading strong={false}>
            Image
          </Heading>
        }
        size="large">
        <Dropzone
          accept="image/jpeg, image/png"
          onDrop={this.onDrop.bind(this)}>
          <p>Please submit one of the following image types:</p>
            <ul>
              <li>JPEG</li>
              <li>PNG</li>
            </ul>
        </Dropzone>
        <img src={this.state.preview_data} />
      </Card>
    );
  }
}
