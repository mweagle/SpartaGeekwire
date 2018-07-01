import React, { Component } from 'react';
import Form from 'grommet/components/Form';
import Footer from 'grommet/components/Footer';
import Card from 'grommet/components/Card';
import Heading from 'grommet/components/Heading';
import FormField from 'grommet/components/FormField';
import TextInput from 'grommet/components/TextInput';
import Button from 'grommet/components/Button';
import {trim} from 'lodash';

export default class ResultsView extends Component {
  constructor() {
    super();
    this.handleCommentChange = this.handleCommentChange.bind(this);
    this.handleSubmit = this.handleSubmit.bind(this);
  }

  handleCommentChange(event) {
    var updatedValue = trim(event.target.value);
    this.setState({
      comment: updatedValue
    });
  }

  handleSubmit(event) {
    event.preventDefault();
    var feedbackBody = {
      lang : "en",
      comment: this.state.comment
    };
    if (this.props.submitCommentHandler) {
      this.props.submitCommentHandler(feedbackBody);
    }
  }

  render() {
    if (!this.props.consolidatedResponse) {
      return null;
    }
    return (
      <Card
        contentPad="large"
        heading={
          <Heading strong={false}>
            Results
          </Heading>
        }
        size="large">
        <Form compact={false}>
          <FormField label='Polly Audio'>
            <audio
              controls src={"data:audio/mp3;base64," + this.props.consolidatedResponse.polly} />
          </FormField>
          <FormField label='What do you think?' size="large">
            <TextInput onDOMChange={this.handleCommentChange.bind(this)}/>
          </FormField>
          <Footer pad={{"vertical": "medium"}}>
            <Button label='Submit'
              type='submit'
              primary={true}
              onClick={this.handleSubmit.bind(this)} />
          </Footer>
        </Form>
      </Card>);
  }
}
