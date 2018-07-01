import React, { Component } from 'react';
import Box from 'grommet/components/Box';
import Header from './Header';
import LabelsCloud from './LabelsCloud';
import manifest_data from './manifest.js';
import axios from 'axios';
import Accept from './Accept';
import Results from './Results';
import Feedback from './Feedback';

export default class PrimaryPage extends Component {
  constructor(props) {
    super(props);

    this.state = {
      manifest: null,
      preview_data : null,
      results_url: null,
      consolidated_response: null,
      feedback: null,
      id_name: null
    };
    this.submitFile = this.submitFile.bind(this);
    this.submitComment = this.submitComment.bind(this);
    this.onPoll = this.onPoll.bind(this);
  }
  componentDidMount() {
    // axios.get(websiteURL + `MANIFEST.json`)
    //   .then(res => {
    //     this.setState({ manifest : res });
    //   });
    this.setState({manifest: manifest_data});
  }

  apigatewayURL(subpath) {
    var baseURL = null;
    if (this.state.manifest &&
        this.state.manifest.APIGatewayURL &&
        this.state.manifest.APIGatewayURL.Value) {
      baseURL = this.state.manifest.APIGatewayURL.Value;
    }
    if (!baseURL) {
      return null;
    }
    return baseURL + "/" + subpath;
  }

  purgeState() {
    this.setState({
      feedback: null,
      preview_data: null,
      consolidated_response: null
    });
  }

  onPoll(consolidatedResponseURL) {
    var self = this;
    var handler = this.onPoll.bind(this);

    var onCallback = function() {
      axios.get(consolidatedResponseURL)
        .then(
          (response) => {
            self.setState({
              consolidated_response: response.data,
              timer_id: null
            });
          },
          (error) => {
            self.setState({
              timer_id: setTimeout(handler,
                1000,
                consolidatedResponseURL)
            });
          }).catch(error => {
            self.setState({
              timer_id: setTimeout(handler,
                1000,
                consolidatedResponseURL)
            });
          });
    };
    if (!this.state.timer_id) {
      self.setState({
        timer_id: setTimeout(handler,
            1000,
            consolidatedResponseURL)
      });
    } else if (!this.state.result_data) {
      onCallback();
    }
  }

  submitFile(selectedFile) {
    this.purgeState();
    var self = this;
    var s3PresignedURL = this.apigatewayURL("presigned");
    if (!s3PresignedURL) {
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      axios.get(s3PresignedURL)
      .then((response) => {
        // Great, get the last element which is the unique ID used
        // for all entities...
        var presignedResponse = response.data;

        self.setState({
          results_url: presignedResponse.results_url
        });
        // Start polling for the response
        self.onPoll(presignedResponse.results_url);

        return axios.request(
          {
            url: presignedResponse.put_object_url,
            method: "PUT",
            data: reader.result,
            headers: { 'content-type': "application/octet-stream" },
            responseType: 'json'
          });
      }).then((response) => {
        console.log('Response', response);
      });
    };
    reader.onabort = () => console.log('file reading was aborted');
    reader.onerror = () => console.log('file reading has failed');
    reader.readAsArrayBuffer(selectedFile);
    // URL to the preview
    // Load the image
    // https://scotch.io/tutorials/use-the-html5-file-api-to-work-with-files-locally-in-the-browser
    // release it
    this.setState({preview_data: selectedFile.preview});
  }

  submitComment(feedbackBody) {
    var self = this;
    this.setState({
      feedback: null
    });
    var commentURL = this.apigatewayURL("feedback");
    if (!commentURL) {
      return;
    }
    axios.post(commentURL, feedbackBody)
      .then(function (response) {
        self.setState({
          feedback: response.data
        });
        console.log(response);
      })
      .catch(function (error) {
        console.log(error);
      });
  }


  render() {
    return (
      <Box>
        <Header />
        <Box direction="row">
          <Accept
            manifest={this.state.manifest}
            submitFileHandler={this.submitFile}/>
          <LabelsCloud
            consolidatedResponse={this.state.consolidated_response} />
          <Results
            consolidatedResponse={this.state.consolidated_response}
            submitCommentHandler={this.submitComment} />
         <Feedback feedback={this.state.feedback} />
        </Box>
      </Box>
    );
  }
};
